package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alphonse927/kpixiv/internal/app"
	"github.com/alphonse927/kpixiv/internal/build"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/gui"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/platform"
	"github.com/alphonse927/kpixiv/internal/tray"

	"github.com/spf13/cobra"
)

var (
	cfgPath string
	verbose bool
	reset   bool
	cfg     *config.Config
)

func runDesktop(*cobra.Command, []string) error {
	log := logger.WithComponent("daemon")

	listener, err := platform.TryAcquire()
	if err != nil {
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

	go func() {
		<-sigCh
		log.Info("Received shutdown signal")
		controller.Shutdown()
		if closeErr := listener.Close(); closeErr != nil {
			log.Error("Failed to close instance listener", "error", closeErr)
		}
		cancel()
	}()

	quitCh := make(chan struct{})
	go func() {
		tray.Run(ctx, controller)
		close(quitCh)
	}()

	log.Info("Starting kPixiv")
	gui.Run(controller, ctx, quitCh)

	log.Info("kPixiv stopped")
	return nil
}

var rootCmd = &cobra.Command{
	Use:     "kpixiv",
	Version: build.Version,
	Short:   "KPixiv - Pixiv wallpaper manager for KDE Plasma",
	Long:    `KPixiv fetches wallpapers from Pixiv and sets them as your KDE Plasma desktop wallpaper.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		cfg.Validate()
		logger.Init(verbose)
		return nil
	},
	RunE: runDesktop,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file (default: ~/.config/kpixiv/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.Flags().BoolVar(&reset, "reset", false, "Remove all cached images before starting")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
