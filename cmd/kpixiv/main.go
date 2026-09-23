package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/alphonse927/kpixiv/internal/app"
	"github.com/alphonse927/kpixiv/internal/auth"
	"github.com/alphonse927/kpixiv/internal/build"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/gui"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/notify"
	"github.com/alphonse927/kpixiv/internal/platform"
	"github.com/alphonse927/kpixiv/internal/tray"

	"github.com/spf13/cobra"
)

var (
	cfgPath    string
	verbose    bool
	reset      bool
	foreground bool
	cfg        *config.Config
)

// runningUnderSystemd reports whether this process was started by systemd
// (service or scope), user or system. systemd sets INVOCATION_ID for every
// unit it starts; it's the standard, portable way to detect this.
func runningUnderSystemd() bool {
	return os.Getenv("INVOCATION_ID") != ""
}

func runDesktop(cmd *cobra.Command, args []string) error {
	log := logger.WithComponent("daemon")

	// kPixiv only supports running as the kpixiv.service systemd user
	// service: that's what gives it autostart, auto-restart on crash, and a
	// single well-defined process. A plain `kpixiv` invocation (double-
	// clicking the app entry, running the binary from a terminal) is not a
	// second, equally valid way to run it -- instead it hands off to the
	// systemd-managed instance and exits, rather than becoming a second,
	// unsupervised process that can drift out of sync with it.
	//
	// --foreground bypasses this hand-off for local debugging: it runs the
	// daemon directly in this terminal, with no auto-restart and no tray/
	// systemd coordination beyond the single-instance lock below.
	if !foreground && !runningUnderSystemd() {
		return handOffToSystemd(log)
	}

	listener, err := platform.TryAcquire()
	if err != nil {
		// This should be rare now that a plain launch hands off to systemd
		// instead of racing it: it mainly means --foreground was used while
		// the systemd-managed instance (or another --foreground run) is
		// already active. Log it and tell the user via a desktop
		// notification so it's clear the app IS running, just not as a
		// second process.
		log.Warn("Another kPixiv instance is already running; exiting", "error", err)
		notify.SendDefault("KPixiv", "kPixiv is already running. Look for it in the system tray.")
		return nil
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			log.Error("Failed to close instance listener", "error", closeErr)
		}
	}()

	controller, err := app.New(cfg, false, reset)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// shutdown tears the app down exactly once, however, it's triggered
	// (SIGTERM, or the user quitting from the tray). Previously, on
	// SIGTERM, cancel() -- which is what wakes up gui.Run() and the tray's
	// event loop -- was only called *after* controller.Shutdown() had
	// fully returned, and Shutdown() blocks until the scheduler's
	// goroutine winds down (including any fetch/bookmark sync already in
	// flight). That meant the GUI and tray sat frozen for the whole
	// scheduler teardown before they even started quitting, on top of
	// their own teardown time. Calling cancel() first lets the GUI and
	// tray start quitting immediately, in parallel with the scheduler
	// stopping, instead of strictly after it.
	var shutdownOnce sync.Once
	shutdownDone := make(chan struct{})
	shutdown := func() {
		shutdownOnce.Do(func() {
			cancel()
			controller.Shutdown()
			close(shutdownDone)
		})
	}

	go func() {
		<-sigCh
		log.Info("Received shutdown signal")
		shutdown()
	}()

	quitCh := make(chan struct{})
	go func() {
		tray.Run(ctx, controller)
		close(quitCh)
	}()

	log.Info("Starting kPixiv")
	gui.Run(controller, ctx, quitCh)

	// gui.Run() can return as soon as cancel() fires, which may be before
	// controller.Shutdown() (started above on SIGTERM, or not started at
	// all if the user quit from the tray instead) has actually finished.
	// Trigger it here too -- a no-op if the signal handler already did --
	// and wait for it, so the process doesn't exit while that teardown is
	// still writing files or mid-request.
	shutdown()
	<-shutdownDone

	log.Info("kPixiv stopped")
	return nil
}

// handOffToSystemd is used when kPixiv is launched directly (not by
// systemd) without --foreground: the app menu entry, a pixiv:// callback
// with no running instance yet, or a user typing `kpixiv` in a terminal out
// of habit. It starts the systemd-managed instance if it isn't already
// running and returns, instead of becoming a second, competing process.
func handOffToSystemd(log *slog.Logger) error {
	const service = "kpixiv.service"

	if !platform.SystemdAvailable() {
		log.Warn("systemctl not found; running in the foreground as a fallback. " +
			"It will not auto-restart or start automatically on login. " +
			"Install systemd, or use --foreground to silence this warning.")
		foreground = true
		return runDesktop(nil, nil)
	}

	if platform.IsServiceActive(service) {
		log.Info("kPixiv is already running as the systemd user service")
		notify.SendDefault("KPixiv", "kPixiv is already running. Look for it in the system tray.")
		return nil
	}

	log.Info("Starting kPixiv as the systemd user service")
	if err := platform.StartService(service); err != nil {
		log.Error("Failed to start the systemd service; running in the foreground as a fallback", "error", err)
		notify.SendDefault("KPixiv", "Could not start the background service; running kPixiv directly instead.")
		foreground = true
		return runDesktop(nil, nil)
	}

	notify.SendDefault("KPixiv", "kPixiv is starting in the background.")
	return nil
}

var rootCmd = &cobra.Command{
	Use:     "kpixiv",
	Version: build.Version,
	Short:   "KPixiv - Pixiv wallpaper manager for KDE Plasma",
	Long:    `KPixiv fetches wallpapers from Pixiv and sets them as your KDE Plasma desktop wallpaper.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		logger.Init(verbose)

		var err error
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("could not load configuration:\n  %w", err)
		}

		issues := cfg.Validate()
		for _, issue := range issues {
			logger.WithComponent("config").Warn("Configuration adjusted", "detail", issue)
		}

		if !verbose {
			logger.SetLevel(cfg.LogLevel)
		}
		return nil
	},
	RunE: runDesktop,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file (default: ~/.config/kpixiv/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.Flags().BoolVar(&reset, "reset", false, "Remove all cached images before starting")
	rootCmd.Flags().BoolVar(&foreground, "foreground", false,
		"Advanced: run directly in this terminal instead of handing off to the systemd user "+
			"service. Intended for debugging -- no auto-restart, no autostart integration.")
}

func main() {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "pixiv://") {
			if err := auth.SendCallback(arg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
