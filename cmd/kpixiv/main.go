package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/fetcher"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"

	"github.com/spf13/cobra"
)

var (
	cfgPath string
	verbose bool
	dryRun  bool
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

		if err = cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

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

		pixivClient, err := pixiv.NewClient()
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
		fmt.Printf("Total: %d, Filtered: %d, Skipped: %d, Failed: %d\n", result.Total, result.Filtered, result.Skipped, result.Failed)
		return nil
	},
}

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Set the next wallpaper in rotation",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.WithComponent("next")

		homeDir, hErr := os.UserHomeDir()
		if hErr != nil {
			return fmt.Errorf("failed to get home directory: %w", hErr)
		}

		s, err := storage.New(homeDir, cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		var setter wallpaper.Setter
		if dryRun {
			setter = wallpaper.NewDryRunSetter()
		} else {
			setter = wallpaper.NewKDESetter()
		}

		// Initializing and loading images queue
		q := storage.NewQueue(s.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		sch := scheduler.New(cfg, s, nil, setter)
		if err := sch.SetNext(q); err != nil {
			return err
		}

		log.Info("Next wallpaper set")
		return nil
	},
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the KPixiv wallpaper daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.WithComponent("daemon")

		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		var setter wallpaper.Setter
		if dryRun {
			setter = wallpaper.NewDryRunSetter()
		} else {
			setter = wallpaper.NewKDESetter()
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		sch := scheduler.New(cfg, st, pixivClient, setter)
		if err := sch.Run(ctx); err != nil {
			return err
		}

		log.Info("KPixiv daemon started")

		if err := sch.ApplyCurrentOrNext(); err != nil {
			log.Warn("Could not apply wallpaper on startup", "error", err)
		}

		<-ctx.Done()
		log.Info("Shutting down daemon")
		sch.Stop()
		log.Info("KPixiv daemon stopped")
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

func main() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file (default: ~/.config/kpixiv/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show actions without applying or downloading")

	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(nextCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
