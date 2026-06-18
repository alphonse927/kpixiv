package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alphonse927/kpixiv/internal/app"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/fetcher"
	"github.com/alphonse927/kpixiv/internal/gui"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/tray"
	"github.com/alphonse927/kpixiv/internal/wallpaper"

	"github.com/spf13/cobra"
)

var (
	cfgPath string
	verbose bool
	dryRun  bool
	reset   bool
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "kpixiv",
	Short: "KPixiv - Pixiv wallpaper daemon for KDE Plasma",
	Long:  `KPixiv fetches wallpapers from Pixiv and sets them as your KDE Plasma desktop wallpaper.`,
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
}

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch wallpapers from Pixiv",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		ctx := context.Background()
		f := fetcher.NewFetcher(cfg, st, pixivClient)
		if err = f.LoadPage(); err != nil {
			return fmt.Errorf("failed to load ranking page: %w", err)
		}

		if dryRun {
			if _, err = f.DryRun(ctx); err != nil {
				return fmt.Errorf("failed to fetch: %w", err)
			}
			return nil
		}

		result, err := f.Fetch(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch: %w", err)
		}

		fmt.Println("Fetch complete!")
		fmt.Printf("Total: %d, Downloaded: %d, Filtered: %d, Skipped: %d, Failed: %d\n", result.Total, result.Downloaded, result.Filtered, result.Skipped, result.Failed)
		return nil
	},
}

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Set the next wallpaper in rotation",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.WithComponent("next")

		s, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		var setter wallpaper.Setter
		if dryRun {
			setter = wallpaper.NewDryRunSetter()
		} else {
			setter = wallpaper.NewKDESetter(cfg.KDE.SetLockScreen)
		}

		// Initializing and loading images queue
		q := storage.NewQueue(s.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		log.Debug("Setting wallpaper")
		sch := scheduler.New(cfg, s, nil, setter)
		if err := sch.SetNextWallpaper(q, "next"); err != nil {
			if errors.Is(err, scheduler.ErrImageNotFound) {
				return nil
			}

			return err
		}
		return nil
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the KPixiv wallpaper daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.WithComponent("daemon")
		controller, err := app.New(cfg, dryRun, reset)
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
			cancel()
		}()

		quitCh := make(chan struct{})
		go func() {
			tray.Run(ctx, controller)
			close(quitCh)
		}()

		log.Info("Starting kPixiv")
		gui.Run(cfg, func() {
			if controller != nil {
				controller.ApplyConfig(cfg)
			}
		}, ctx, quitCh)

		log.Info("kPixiv stopped")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current KPixiv status",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.WithComponent("status")

		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		history, err := st.LoadHistory()
		if err != nil {
			return fmt.Errorf("failed to load history: %w", err)
		}

		metadata, err := st.LoadMetadata()
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}

		rankingEntries, err := os.ReadDir(st.RankingDir())
		if err != nil {
			return fmt.Errorf("failed to read ranking directory: %w", err)
		}

		totalWallpapers := 0
		for _, entry := range rankingEntries {
			if entry.IsDir() {
				continue
			}
			totalWallpapers++
		}

		fmt.Println("=== KPixiv Status ===")
		fmt.Printf("Config file: %s\n", cfgPath)
		fmt.Printf("Download directory: %s\n", st.DownloadDir())
		fmt.Printf("Data directory: %s\n", st.DataDir())
		fmt.Printf("Set interval: %d minutes\n", cfg.Wallpaper.SetInterval)
		fmt.Printf("Fetch interval: %d minutes\n", cfg.Wallpaper.FetchInterval)
		fmt.Printf("History limit: %d\n", cfg.Wallpaper.HistoryLimit)
		fmt.Printf("Cleanup images older than: %d days\n", cfg.Wallpaper.CleanupDays)
		fmt.Printf("\n=== Wallpaper History ===\n")
		fmt.Printf("Total wallpapers: %d\n", totalWallpapers)
		fmt.Printf("Current: %s\n", history.Current)
		fmt.Printf("Last updated: %s\n", history.UpdatedAt.Format(time.DateTime))
		fmt.Printf("\n=== Storage ===\n")
		fmt.Printf("Downloaded images: %d\n", len(metadata))

		log.Debug("Status displayed")
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file (default: ~/.config/kpixiv/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show actions without applying or downloading")
	daemonCmd.Flags().BoolVar(&reset, "reset", false, "Remove all cached images before daemon starts")

	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(nextCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(statusCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
