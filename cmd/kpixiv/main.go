package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alphonse927/kpixiv/internal/app"
	"github.com/alphonse927/kpixiv/internal/bookmarks"
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
		gui.Run(controller, ctx, quitCh)

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
		fmt.Printf("Ranking feed: %s\n", cfg.Pixiv.Ranking)
		fmt.Printf("Set interval: %d minutes\n", cfg.Wallpaper.SetInterval)
		fmt.Printf("Fetch interval: %d minutes\n", cfg.Wallpaper.FetchInterval)
		fmt.Printf("Min image size: %dx%d\n", cfg.Pixiv.MinWidth, cfg.Pixiv.MinHeight)
		fmt.Printf("R-18: %t\n", cfg.Pixiv.R18)
		fmt.Printf("Landscape only: %t\n", cfg.Pixiv.LandscapeOnly)
		fmt.Printf("Wallpaper history: %d\n", cfg.Wallpaper.HistoryLimit)
		fmt.Printf("Cleanup images older than: %d days\n", cfg.Wallpaper.CleanupDays)
		fmt.Printf("Lock screen: %t\n", cfg.KDE.SetLockScreen)
		fmt.Printf("\n=== Wallpaper History ===\n")
		fmt.Printf("Total wallpapers: %d\n", totalWallpapers)
		fmt.Printf("Current: %s\n", history.Current)
		if len(history.Images) > 0 {
			fmt.Printf("Previous: %s\n", history.Images[len(history.Images)-1])
		} else {
			fmt.Printf("Previous: none\n")
		}
		fmt.Printf("Last updated: %s\n", history.UpdatedAt.Format(time.DateTime))
		fmt.Printf("\n=== Storage ===\n")
		fmt.Printf("Downloaded images: %d\n", len(metadata))

		q := storage.NewQueue(st.StateDir())
		if err := q.Load(); err == nil {
			fmt.Printf("In queue: %d\n", q.Len())
		}

		log.Debug("Status displayed")
		return nil
	},
}

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the wallpaper queue",
}

var queueFavoritesCmd = &cobra.Command{
	Use:   "favorites",
	Short: "Clear the queue and load images from the Favorites folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		if err := q.Clear(); err != nil {
			return fmt.Errorf("failed to clear queue: %w", err)
		}

		blacklist, err := st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("failed to load blacklist: %w", err)
		}

		ids := scanDirForImages(st.FavoritesDir(), blacklist)
		if len(ids) == 0 {
			fmt.Println("No images found in Favorites folder")
			return nil
		}

		if err := q.AppendRandom(ids); err != nil {
			return fmt.Errorf("failed to populate queue: %w", err)
		}

		fmt.Printf("Queue rebuilt: %d images loaded from Favorites\n", len(ids))
		return nil
	},
}

var queueRankingCmd = &cobra.Command{
	Use:   "ranking",
	Short: "Clear the queue and load images from the Ranking folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		if err := q.Clear(); err != nil {
			return fmt.Errorf("failed to clear queue: %w", err)
		}

		blacklist, err := st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("failed to load blacklist: %w", err)
		}

		ids := scanDirForImages(st.RankingDir(), blacklist)
		if len(ids) == 0 {
			fmt.Println("No images found in Ranking folder")
			return nil
		}

		if err := q.AppendRandom(ids); err != nil {
			return fmt.Errorf("failed to populate queue: %w", err)
		}

		fmt.Printf("Queue rebuilt: %d images loaded from Ranking\n", len(ids))
		return nil
	},
}

var queueAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Clear the queue and load images from both Ranking and Favorites folders",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		if err := q.Clear(); err != nil {
			return fmt.Errorf("failed to clear queue: %w", err)
		}

		blacklist, err := st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("failed to load blacklist: %w", err)
		}

		rankingIDs := scanDirForImages(st.RankingDir(), blacklist)
		favoritesIDs := scanDirForImages(st.FavoritesDir(), blacklist)

		all := make([]string, 0, len(rankingIDs)+len(favoritesIDs))
		seen := make(map[string]bool)
		for _, id := range rankingIDs {
			if !seen[id] {
				all = append(all, id)
				seen[id] = true
			}
		}
		for _, id := range favoritesIDs {
			if !seen[id] {
				all = append(all, id)
				seen[id] = true
			}
		}

		if len(all) == 0 {
			fmt.Println("No images found in Ranking or Favorites folders")
			return nil
		}

		if err := q.AppendRandom(all); err != nil {
			return fmt.Errorf("failed to populate queue: %w", err)
		}

		fmt.Printf("Queue rebuilt: %d images loaded from Ranking and Favorites\n", len(all))
		return nil
	},
}

func scanDirForImages(dir string, blacklist map[string]struct{}) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			continue
		}

		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}

		id := strings.TrimSuffix(name, filepath.Ext(name))
		if _, excluded := blacklist[id]; excluded {
			continue
		}

		ids = append(ids, id)
	}

	return ids
}

var bookmarksCmd = &cobra.Command{
	Use:   "bookmarks",
	Short: "Manage Pixiv bookmarks",
}

var bookmarksSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync bookmarked images from Pixiv",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		if !pixivClient.LoggedIn() {
			return fmt.Errorf("you must be logged in to sync your bookmarks")
		}

		ctx := context.Background()
		syncer := bookmarks.NewSyncer(cfg, st, pixivClient)
		result, err := syncer.Sync(ctx)
		if err != nil {
			return fmt.Errorf("failed to sync bookmarks: %w", err)
		}

		fmt.Println("Bookmark sync complete!")
		fmt.Printf("Total: %d, Downloaded: %d, Deleted: %d, Skipped: %d, Failed: %d\n", result.Total, result.Downloaded, result.Deleted, result.Skipped, result.Failed)
		return nil
	},
}

var bookmarksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List locally bookmarked images",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		bookmarkIDs, err := st.LoadBookmarks()
		if err != nil {
			return fmt.Errorf("failed to load bookmarks: %w", err)
		}

		metadata, err := st.LoadMetadata()
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}

		if len(bookmarkIDs) == 0 {
			fmt.Println("No bookmarks found.")
			return nil
		}

		fmt.Println("=== Bookmarked Images ===")
		for id := range bookmarkIDs {
			if meta, ok := metadata[id]; ok {
				fmt.Printf("  %s - %s by %s [%dx%d]\n", id, meta.Title, meta.Artist, meta.Width, meta.Height)
			} else {
				fmt.Printf("  %s (not downloaded)\n", id)
			}
		}
		fmt.Printf("\nTotal bookmarks: %d\n", len(bookmarkIDs))
		return nil
	},
}

var bookmarksAddCmd = &cobra.Command{
	Use:   "add <illust_id>",
	Short: "Bookmark an artwork on Pixiv",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		illustID := strings.TrimSpace(args[0])
		if illustID == "" {
			return fmt.Errorf("illust ID is required")
		}

		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		if !pixivClient.LoggedIn() {
			return fmt.Errorf("you must be logged in to bookmark artwork")
		}

		ctx := context.Background()
		if err := pixivClient.BookmarkIllust(ctx, illustID); err != nil {
			return fmt.Errorf("failed to bookmark on pixiv: %w", err)
		}

		if err := st.AddBookmark(illustID); err != nil {
			return fmt.Errorf("failed to save local bookmark: %w", err)
		}

		fmt.Printf("Artwork %s bookmarked successfully\n", illustID)
		return nil
	},
}

var bookmarksAddCurrentCmd = &cobra.Command{
	Use:   "add-current",
	Short: "Bookmark the current wallpaper on Pixiv",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		currentID, err := st.GetCurrentWallpaper()
		if err != nil {
			return fmt.Errorf("failed to get current wallpaper: %w", err)
		}
		if currentID == "" {
			return fmt.Errorf("no current wallpaper")
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		if !pixivClient.LoggedIn() {
			return fmt.Errorf("you must be logged in to bookmark artwork")
		}

		ctx := context.Background()
		if err := pixivClient.BookmarkIllust(ctx, currentID); err != nil {
			return fmt.Errorf("failed to bookmark on pixiv: %w", err)
		}

		if err := st.AddBookmark(currentID); err != nil {
			return fmt.Errorf("failed to save local bookmark: %w", err)
		}

		fmt.Printf("Current artwork %s bookmarked successfully\n", currentID)
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
	rootCmd.AddCommand(queueCmd)
	rootCmd.AddCommand(bookmarksCmd)

	queueCmd.AddCommand(queueRankingCmd)
	queueCmd.AddCommand(queueFavoritesCmd)
	queueCmd.AddCommand(queueAllCmd)

	bookmarksCmd.AddCommand(bookmarksSyncCmd)
	bookmarksCmd.AddCommand(bookmarksListCmd)
	bookmarksCmd.AddCommand(bookmarksAddCmd)
	bookmarksCmd.AddCommand(bookmarksAddCurrentCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
