package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kpixiv/kpixiv/internal/cache"
	"github.com/kpixiv/kpixiv/internal/config"
	"github.com/kpixiv/kpixiv/internal/logger"
	"github.com/kpixiv/kpixiv/internal/pixiv"
	"github.com/kpixiv/kpixiv/internal/scheduler"
	"github.com/kpixiv/kpixiv/internal/storage"
	"github.com/kpixiv/kpixiv/internal/wallpaper"

	"github.com/spf13/cobra"
)

var (
	cfgPath   string
	verbose   bool
	cfg       *config.Config
	sched     *scheduler.Scheduler
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

		if err := cfg.Validate(); err != nil {
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
		log := logger.WithComponent("fetch")

		st, err := storage.New(cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}
		c := cache.NewCache(st)

		ctx := context.Background()
		rankingType := pixiv.RankingType(cfg.Pixiv.Ranking)

		images, err := pixivClient.FetchRanking(ctx, rankingType, 1, cfg.Pixiv.R18)
		if err != nil {
			return fmt.Errorf("failed to fetch rankings: %w", err)
		}

		filtered := 0
		for _, img := range images {
			if img.Width < cfg.Pixiv.MinWidth || img.Height < cfg.Pixiv.MinHeight {
				continue
			}
			if cfg.Pixiv.LandscapeOnly && img.Height > img.Width {
				continue
			}
			filtered++
		}

		log.Info("Filtered images", "count", filtered, "minWidth", cfg.Pixiv.MinWidth, "minHeight", cfg.Pixiv.MinHeight, "landscapeOnly", cfg.Pixiv.LandscapeOnly)

		c.Add(images)

		fmt.Println("Fetch complete!")
		fmt.Printf("Total: %d, Filtered: %d\n", len(images), filtered)

		return nil
	},
}

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Set the next wallpaper in rotation",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.WithComponent("next")

		st, err := storage.New(cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		setter := wallpaper.NewKDEBackgroundSetter()

		pixivClient, err := pixiv.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}
		c := cache.NewCache(st)

		sch := scheduler.New(cfg, st, c, pixivClient, setter)

		if err := sch.SetNext(context.Background()); err != nil {
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

		st, err := storage.New(cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}
		c := cache.NewCache(st)
		setter := wallpaper.NewKDEBackgroundSetter()

		sch := scheduler.New(cfg, st, c, pixivClient, setter)

		ctx, cancel := context.WithCancel(context.Background())

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigCh
			log.Info("Received shutdown signal")
			cancel()
		}()

		if err := sch.Run(ctx); err != nil {
			return err
		}

		log.Info("KPixiv daemon started")
		fmt.Println("KPixiv daemon running... Press Ctrl+C to stop")

		<-ctx.Done()
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

		st, err := storage.New(cfg.DownloadPath)
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

		fmt.Println("=== KPixiv Status ===")
		fmt.Printf("Config file: %s\n", cfgPath)
		fmt.Printf("Download directory: %s\n", st.DownloadDir())
		fmt.Printf("Data directory: %s\n", st.DataDir())
		fmt.Printf("Interval: %d minutes\n", cfg.IntervalMinutes)
		fmt.Printf("\n=== Wallpaper History ===\n")
		fmt.Printf("Total wallpapers: %d\n", len(history.Images))
		fmt.Printf("Current: %s\n", history.Current)
		fmt.Printf("Last updated: %s\n", history.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("\n=== Storage ===\n")
		fmt.Printf("Downloaded images: %d\n", len(metadata))

		log.Info("Status displayed")
		return nil
	},
}

func main() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file (default: ~/.config/kpixiv/config.toml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(nextCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}