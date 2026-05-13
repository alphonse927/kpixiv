package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

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
	cfgPath string
	verbose bool
	dryRun  bool
	cfg     *config.Config
	sched   *scheduler.Scheduler
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
		pageKey := fmt.Sprintf("%s:%t", cfg.Pixiv.Ranking, cfg.Pixiv.R18)
		page, err := st.GetRankingPage(pageKey)
		if err != nil {
			return fmt.Errorf("failed to load ranking page: %w", err)
		}

		images, nextPage, err := pixivClient.FetchRanking(ctx, rankingType, page, cfg.Pixiv.R18)
		if err != nil {
			return fmt.Errorf("failed to fetch rankings: %w", err)
		}

		if err := st.SetRankingPage(pageKey, nextPage); err != nil {
			return fmt.Errorf("failed to save next ranking page: %w", err)
		}

		filtered := 0
		filteredImages := []pixiv.Image{}
		for _, img := range images {
			if img.Width < cfg.Pixiv.MinWidth || img.Height < cfg.Pixiv.MinHeight {
				continue
			}
			if cfg.Pixiv.LandscapeOnly && img.Height > img.Width {
				continue
			}
			filtered++
			filteredImages = append(filteredImages, img)
		}

		log.Debug("Filtered images", "count", filtered, "minWidth", cfg.Pixiv.MinWidth, "minHeight", cfg.Pixiv.MinHeight, "landscapeOnly", cfg.Pixiv.LandscapeOnly)
		if dryRun {
			log.Info("Dry-run mode: skipping downloads", "candidates", len(filteredImages))
			fmt.Println("Fetch dry-run complete!")
			fmt.Printf("Total: %d, Filtered: %d, Downloaded: 0, Skipped: 0\n", len(images), filtered)
			return nil
		}

		rankingDir := st.RankingDir()
		metadata, err := st.LoadMetadata()
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}

		downloaded := 0
		skipped := 0

		pending := make([]pixiv.Image, 0, len(filteredImages))
		for _, img := range filteredImages {
			if existing, ok := metadata[img.ID]; ok {
				if _, err := os.Stat(existing.Path); err == nil {
					skipped++
					log.Debug("Skipping existing image from metadata", "id", img.ID, "path", existing.Path)
					continue
				}
			}

			destPath := filepath.Join(rankingDir, img.ID+".jpg")
			altPath := filepath.Join(rankingDir, img.ID+".png")

			if _, err := os.Stat(destPath); err == nil {
				skipped++
				metadata[img.ID] = storage.ImageMeta{
					ID:           img.ID,
					Path:         destPath,
					Width:        img.Width,
					Height:       img.Height,
					Title:        img.Title,
					Artist:       img.Artist,
					ArtistID:     img.ArtistID,
					DownloadedAt: time.Now(),
				}
				continue
			}

			if _, err := os.Stat(altPath); err == nil {
				skipped++
				metadata[img.ID] = storage.ImageMeta{
					ID:           img.ID,
					Path:         altPath,
					Width:        img.Width,
					Height:       img.Height,
					Title:        img.Title,
					Artist:       img.Artist,
					ArtistID:     img.ArtistID,
					DownloadedAt: time.Now(),
				}
				continue
			}

			pending = append(pending, img)
		}

		type downloadResult struct {
			img     pixiv.Image
			path    string
			err     error
			skipped bool
		}

		workers := runtime.NumCPU()
		if workers > 4 {
			workers = 4
		}
		if workers < 1 {
			workers = 1
		}

		jobs := make(chan pixiv.Image)
		results := make(chan downloadResult, len(pending))
		var wg sync.WaitGroup

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for img := range jobs {
					destPath := filepath.Join(rankingDir, img.ID+".jpg")
					altPath := filepath.Join(rankingDir, img.ID+".png")

					if _, err := os.Stat(destPath); err == nil {
						results <- downloadResult{img: img, path: destPath, skipped: true}
						continue
					}
					if _, err := os.Stat(altPath); err == nil {
						results <- downloadResult{img: img, path: altPath, skipped: true}
						continue
					}

					if err := pixivClient.DownloadImage(ctx, img, destPath); err != nil {
						results <- downloadResult{img: img, err: err}
						continue
					}

					finalPath := destPath
					if _, err := os.Stat(destPath); os.IsNotExist(err) {
						if _, err := os.Stat(altPath); err == nil {
							finalPath = altPath
						}
					}

					results <- downloadResult{img: img, path: finalPath}
				}
			}()
		}

		go func() {
			for _, img := range pending {
				jobs <- img
			}
			close(jobs)
			wg.Wait()
			close(results)
		}()

		for result := range results {
			if result.err != nil {
				log.Warn("Failed to download image", "id", result.img.ID, "error", result.err)
				continue
			}

			if result.skipped {
				skipped++
			}

			metadata[result.img.ID] = storage.ImageMeta{
				ID:           result.img.ID,
				Path:         result.path,
				Width:        result.img.Width,
				Height:       result.img.Height,
				Title:        result.img.Title,
				Artist:       result.img.Artist,
				ArtistID:     result.img.ArtistID,
				DownloadedAt: time.Now(),
			}
			if !result.skipped {
				downloaded++
			}
		}

		if err := st.SaveMetadata(metadata); err != nil {
			return fmt.Errorf("failed to save metadata: %w", err)
		}

		log.Info("Download summary", "downloaded", downloaded, "skipped", skipped)

		c.Add(images)

		fmt.Println("Fetch complete!")
		fmt.Printf("Total: %d, Filtered: %d, Downloaded: %d, Skipped: %d\n", len(images), filtered, downloaded, skipped)

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

		var setter wallpaper.Setter
		if dryRun {
			setter = wallpaper.NewDryRunSetter()
		} else {
			setter = wallpaper.NewKDEBackgroundSetter()
		}

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
		var setter wallpaper.Setter
		if dryRun {
			setter = wallpaper.NewDryRunSetter()
		} else {
			setter = wallpaper.NewKDEBackgroundSetter()
		}

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
		fmt.Printf("Interval: %d minutes\n", cfg.IntervalMinutes)
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
