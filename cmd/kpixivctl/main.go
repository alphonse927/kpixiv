package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/auth"
	"github.com/alphonse927/kpixiv/internal/bookmarks"
	"github.com/alphonse927/kpixiv/internal/build"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/fetcher"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/platform"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"

	"github.com/spf13/cobra"
)

var (
	cfgPath     string
	verbose     bool
	dryRun      bool
	monitorID   string
	allMonitors bool
	cfg         *config.Config
)

var rootCmd = &cobra.Command{
	Use:     "kpixivctl",
	Version: build.Version,
	Short:   "KPixiv CLI - Pixiv wallpaper manager",
	Long:    `kpixivctl is the command-line interface for KPixiv, a Pixiv wallpaper manager for KDE Plasma.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		cfg.Validate()

		logger.Init(verbose)
		if !verbose {
			logger.SetLevel(cfg.LogLevel)
		}
		return nil
	},
}

var wallpaperCmd = &cobra.Command{
	Use:   "wallpaper",
	Short: "Manage wallpapers and the rotation queue",
}

var wallpaperFetchCmd = &cobra.Command{
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

var wallpaperNextCmd = &cobra.Command{
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

		q := storage.NewQueue(s.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		log.Debug("Setting wallpaper")
		sch := scheduler.New(cfg, s, nil, setter)
		var setErr error
		switch {
		case monitorID != "":
			screen := sch.ResolveScreen(monitorID)
			if screen == nil {
				return fmt.Errorf("monitor %q not found — use a connector name (e.g. DP-1) or numeric index from 'kpixivctl monitors'", monitorID)
			}
			setErr = sch.SetNextWallpaperForScreen(q, screen.ID, screen.Index, "next")
		case allMonitors:
			setErr = sch.SetNextWallpapers("next")
		default:
			setErr = sch.SetNextWallpaper(q, "next")
		}
		if err := setErr; err != nil {
			if errors.Is(err, scheduler.ErrImageNotFound) {
				return nil
			}

			return err
		}
		return nil
	},
}

var wallpaperQueueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the wallpaper queue",
}

var wallpaperQueueBookmarksCmd = &cobra.Command{
	Use:     "bookmarks",
	Aliases: []string{"favorites"},
	Short:   "Clear the queue and load images from the Bookmarks folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err = q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		if err = q.Clear(); err != nil {
			return fmt.Errorf("failed to clear queue: %w", err)
		}

		var blacklist map[string]struct{}
		blacklist, err = st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("failed to load blacklist: %w", err)
		}

		ids := scanDirForImages(st.BookmarksDir(), blacklist)
		if len(ids) == 0 {
			fmt.Println("No images found in Bookmarks folder")
			return nil
		}

		if err := st.AddBookmarks(ids); err != nil {
			return fmt.Errorf("failed to update bookmarks: %w", err)
		}

		if err := q.AppendRandom(ids); err != nil {
			return fmt.Errorf("failed to populate queue: %w", err)
		}

		fmt.Printf("Queue rebuilt: %d images loaded from Bookmarks\n", len(ids))
		return nil
	},
}

var wallpaperQueueRankingCmd = &cobra.Command{
	Use:   "ranking",
	Short: "Clear the queue and load images from the Ranking folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err = q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		if err = q.Clear(); err != nil {
			return fmt.Errorf("failed to clear queue: %w", err)
		}

		var blacklist map[string]struct{}
		blacklist, err = st.LoadBlacklistSet()
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

var wallpaperQueueAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Clear the queue and load images from both Ranking and Bookmarks folders",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err = q.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}

		if err = q.Clear(); err != nil {
			return fmt.Errorf("failed to clear queue: %w", err)
		}

		var blacklist map[string]struct{}
		blacklist, err = st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("failed to load blacklist: %w", err)
		}

		rankingIDs := scanDirForImages(st.RankingDir(), blacklist)
		bookmarksIDs := scanDirForImages(st.BookmarksDir(), blacklist)

		if len(bookmarksIDs) > 0 {
			if err := st.AddBookmarks(bookmarksIDs); err != nil {
				return fmt.Errorf("failed to update bookmarks: %w", err)
			}
		}

		all := make([]string, 0, len(rankingIDs)+len(bookmarksIDs))
		seen := make(map[string]bool)
		for _, id := range rankingIDs {
			if !seen[id] {
				all = append(all, id)
				seen[id] = true
			}
		}
		for _, id := range bookmarksIDs {
			if !seen[id] {
				all = append(all, id)
				seen[id] = true
			}
		}

		if len(all) == 0 {
			fmt.Println("No images found in Ranking or Bookmarks folders")
			return nil
		}

		if err := q.AppendRandom(all); err != nil {
			return fmt.Errorf("failed to populate queue: %w", err)
		}

		fmt.Printf("Queue rebuilt: %d images loaded from Ranking and Bookmarks\n", len(all))
		return nil
	},
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
	Long: `Bookmark the current wallpaper on Pixiv and save it locally.

With --monitor, bookmarks the current wallpaper for a specific screen.
With --all, bookmarks the current wallpaper on every active screen.`,
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
			return fmt.Errorf("you must be logged in to bookmark artwork")
		}

		ctx := context.Background()

		var currentIDs []string
		switch {
		case monitorID != "":
			setter := wallpaper.NewKDESetter(false)
			screens, listErr := setter.Screens()
			if listErr == nil {
				for _, s := range screens {
					if s.ID == monitorID || s.Index == monitorID {
						monitorID = s.ID
						break
					}
				}
			}

			monitors, err := st.LoadMonitorHistory()
			if err != nil {
				return fmt.Errorf("failed to load monitor history: %w", err)
			}

			id, ok := monitors[monitorID]
			if !ok || id == "" {
				return fmt.Errorf("no current wallpaper for monitor %s", monitorID)
			}

			currentIDs = []string{id}

		case allMonitors:
			setter := wallpaper.NewKDESetter(false)
			screens, err := setter.Screens()
			if err != nil {
				return fmt.Errorf("failed to list monitors: %w", err)
			}

			monitors, err := st.LoadMonitorHistory()
			if err != nil {
				return fmt.Errorf("failed to load monitor history: %w", err)
			}

			globalID, err := st.GetCurrentWallpaper()
			if err != nil {
				globalID = ""
			}

			for _, s := range screens {
				id := monitors[s.ID]
				if id == "" {
					id = globalID
				}
				if id != "" {
					currentIDs = append(currentIDs, id)
				}
			}

			if len(currentIDs) == 0 {
				return fmt.Errorf("no current wallpaper on any monitor")
			}

		default:
			currentID, err := st.GetCurrentWallpaper()
			if err != nil {
				return fmt.Errorf("failed to get current wallpaper: %w", err)
			}
			if currentID == "" {
				return fmt.Errorf("no current wallpaper")
			}
			currentIDs = []string{currentID}
		}

		for _, id := range currentIDs {
			if err := pixivClient.BookmarkIllust(ctx, id); err != nil {
				return fmt.Errorf("failed to bookmark artwork %s on pixiv: %w", id, err)
			}

			if err := st.AddBookmark(id); err != nil {
				return fmt.Errorf("failed to save local bookmark for %s: %w", id, err)
			}

			fmt.Printf("Artwork %s bookmarked successfully\n", id)
		}

		return nil
	},
}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage Pixiv account",
}

var accountLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Pixiv",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		fmt.Println("Opening browser for Pixiv authentication...")
		_, err = auth.Login(context.Background(), auth.LoginConfig{}, &pixiv.AuthProvider{Client: pixivClient})
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		fmt.Println("Login successful!")
		return nil
	},
}

var accountLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from Pixiv",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		if err := pixivClient.Logout(); err != nil {
			return fmt.Errorf("failed to log out: %w", err)
		}

		fmt.Println("Logged out.")
		return nil
	},
}

var accountStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Pixiv login status",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("failed to initialize pixiv client: %w", err)
		}

		if pixivClient.LoggedIn() {
			userName := pixivClient.AuthUserName()
			if userName != "" {
				fmt.Printf("Logged in as: %s\n", userName)
			} else {
				fmt.Println("Logged in to Pixiv")
			}
		} else {
			fmt.Println("Not logged in")
		}

		return nil
	},
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage downloaded image cache",
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		metadata, err := st.LoadMetadata()
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}

		fmt.Printf("Downloaded images: %d\n", len(metadata))

		q := storage.NewQueue(st.StateDir())
		if err := q.Load(); err == nil {
			fmt.Printf("In queue: %d\n", q.Len())
		}

		return nil
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all cached images",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		removed, err := st.CleanupImagesOlderThanDays(0)
		if err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}

		fmt.Printf("Removed %d images\n", removed)
		return nil
	},
}

var autostartCmd = &cobra.Command{
	Use:   "autostart",
	Short: "Manage systemd autostart",
}

var autostartEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Install and enable the systemd user service",
	Long: `Installs the systemd unit file and enables the KPixiv user service.

The unit file is written to ~/.config/systemd/user/kpixiv.service
and the service is enabled for automatic startup on login.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := platform.EnableService("kpixiv.service"); err != nil {
			return err
		}
		fmt.Println("KPixiv autostart enabled.")
		return nil
	},
}

var autostartDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable automatic startup",
	Long: `Prevents KPixiv from starting automatically when you log in.

The current session is not affected. The service unit file is
kept on disk so the service state remains queryable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := platform.DisableService("kpixiv.service"); err != nil {
			return err
		}
		fmt.Println("KPixiv autostart disabled.")
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and edit configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(cfg.ConfigPath)
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}

		fmt.Print(string(data))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value by dotted key path.

Supported keys:
  log_level                   (string: info/debug/warn/error)
  pixiv.r18                   (bool)
  pixiv.ranking               (int: 0=daily, 1=weekly, 2=monthly)
  pixiv.min_width             (int, minimum 1280)
  pixiv.min_height            (int, minimum 720)
  pixiv.landscape_only        (bool)
  wallpaper.set_interval      (int, minutes, minimum 5)
  wallpaper.fetch_interval    (int, minutes, minimum 30)
  wallpaper.history_limit     (int, minimum 1)
  wallpaper.cleanup_days      (int, minimum 1)
  wallpaper.rotation_enabled  (bool)
  wallpaper.fetch_enabled     (bool)
  wallpaper.multi_monitor_enabled (bool)
  kde.set_lock_screen         (bool)
  bookmarks.enabled           (bool)
  bookmarks.sync_interval     (int, minutes, minimum 60)
  bookmarks.auto_cleanup      (bool)
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		switch key {
		case "log_level":
			switch value {
			case "debug", "info", "warn", "error":
			default:
				return fmt.Errorf("invalid log level: %s (must be one of: debug, info, warn, error)", value)
			}
			cfg.LogLevel = value

		case "pixiv.r18":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Pixiv.R18 = v

		case "pixiv.landscape_only":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Pixiv.LandscapeOnly = v

		case "pixiv.ranking":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Pixiv.Ranking = config.RankingMode(v)

		case "pixiv.min_width":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Pixiv.MinWidth = v

		case "pixiv.min_height":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Pixiv.MinHeight = v

		case "wallpaper.set_interval":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Wallpaper.SetInterval = v

		case "wallpaper.fetch_interval":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Wallpaper.FetchInterval = v

		case "wallpaper.history_limit":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Wallpaper.HistoryLimit = v

		case "wallpaper.cleanup_days":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Wallpaper.CleanupDays = v

		case "wallpaper.rotation_enabled":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Wallpaper.RotationEnabled = v

		case "wallpaper.fetch_enabled":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Wallpaper.FetchEnabled = v

		case "wallpaper.multi_monitor_enabled":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Wallpaper.MultiMonitorEnabled = v

		case "kde.set_lock_screen":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.KDE.SetLockScreen = v

		case "bookmarks.enabled":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Bookmarks.Enabled = v

		case "bookmarks.sync_interval":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int value for %s: %s", key, value)
			}
			cfg.Bookmarks.SyncInterval = v

		case "bookmarks.auto_cleanup":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool value for %s: %s", key, value)
			}
			cfg.Bookmarks.AutoCleanup = v

		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		cfg.Validate()
		if err := config.Save(cfg.ConfigPath, cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

var monitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "List active KDE Plasma screens",
	RunE: func(cmd *cobra.Command, args []string) error {
		if dryRun {
			fmt.Println("Dry-run does not query Plasma screens")
			return nil
		}

		screens, err := wallpaper.NewKDESetter(cfg.KDE.SetLockScreen).Screens()
		if err != nil {
			return err
		}

		if len(screens) == 0 {
			fmt.Println("No active Plasma screens found")
			return nil
		}

		for _, screen := range screens {
			state := "enabled"
			if settings, ok := cfg.Wallpaper.Monitors[screen.ID]; ok && !settings.RotationEnabled {
				state = "disabled"
			}

			name := screen.Name
			if name == "" {
				name = "Screen " + screen.ID
			}

			fmt.Printf("[%s] ", screen.Index)
			if screen.Model != "" {
				fmt.Printf("%s (%s/%s)\t%s\n", name, screen.Model, screen.ID, state)
			} else {
				fmt.Printf("%s (%s)\t%s\n", name, screen.ID, state)
			}
		}

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
		monitorHistory, monitorHistoryErr := st.LoadMonitorHistory()

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
		fmt.Printf("Wallpaper history: %d\n", cfg.Wallpaper.HistoryLimit)
		fmt.Printf("Cleanup images older than: %d days\n", cfg.Wallpaper.CleanupDays)
		fmt.Printf("Lock screen: %t\n", cfg.KDE.SetLockScreen)
		fmt.Printf("Multi-monitor: %t\n", cfg.Wallpaper.MultiMonitorEnabled)

		screens, err := wallpaper.NewKDESetter(false).Screens()
		if err != nil {
			screens = nil
		}

		fmt.Printf("\n=== Wallpaper History ===\n")
		fmt.Printf("Total wallpapers: %d\n", totalWallpapers)
		if cfg.Wallpaper.MultiMonitorEnabled {
			if monitorHistoryErr == nil {
				for _, s := range screens {
					imageID, ok := monitorHistory[s.ID]
					if !ok {
						imageID = "none"
					}

					fmt.Printf("Current [%s]: %s\n", s.ID, imageID)
				}
			}
		} else {
			fmt.Printf("Current: %s\n", history.Current)
			if len(history.Images) > 0 {
				fmt.Printf("Previous: %s\n", history.Images[len(history.Images)-1])
			} else {
				fmt.Printf("Previous: none\n")
			}
		}

		fmt.Printf("Last updated: %s\n", history.UpdatedAt.Format(time.DateTime))
		if len(screens) > 0 {
			fmt.Printf("\n=== Monitors ===\n")
			for _, s := range screens {
				model := s.Model
				if model == "" {
					model = "(unknown)"
				}

				settings, configured := cfg.Wallpaper.Monitors[s.ID]
				rotation := "enabled"
				if configured && !settings.RotationEnabled {
					rotation = "disabled"
				}

				orientation := settings.Orientation
				if orientation == "" {
					orientation = "any"
				}

				fmt.Printf("  [%s] %s (%s)\trotation=%s\torientation=%s\n", s.Index, s.ID, model, rotation, orientation)
			}
		}
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

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file (default: ~/.config/kpixiv/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show actions without applying or downloading")

	wallpaperNextCmd.Flags().StringVar(&monitorID, "monitor", "", "set the next wallpaper on one screen ID")
	wallpaperNextCmd.Flags().BoolVar(&allMonitors, "all", false, "set the next wallpaper on every active screen")
	bookmarksAddCurrentCmd.Flags().StringVar(&monitorID, "monitor", "", "bookmark current wallpaper on one screen")
	bookmarksAddCurrentCmd.Flags().BoolVar(&allMonitors, "all", false, "bookmark current wallpaper on every active screen")

	wallpaperQueueCmd.AddCommand(wallpaperQueueRankingCmd)
	wallpaperQueueCmd.AddCommand(wallpaperQueueBookmarksCmd)
	wallpaperQueueCmd.AddCommand(wallpaperQueueAllCmd)

	wallpaperCmd.AddCommand(wallpaperFetchCmd)
	wallpaperCmd.AddCommand(wallpaperNextCmd)
	wallpaperCmd.AddCommand(wallpaperQueueCmd)

	bookmarksCmd.AddCommand(bookmarksSyncCmd)
	bookmarksCmd.AddCommand(bookmarksListCmd)
	bookmarksCmd.AddCommand(bookmarksAddCmd)
	bookmarksCmd.AddCommand(bookmarksAddCurrentCmd)

	accountCmd.AddCommand(accountLoginCmd)
	accountCmd.AddCommand(accountLogoutCmd)
	accountCmd.AddCommand(accountStatusCmd)

	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheClearCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)

	autostartCmd.AddCommand(autostartEnableCmd)
	autostartCmd.AddCommand(autostartDisableCmd)

	rootCmd.AddCommand(wallpaperCmd)
	rootCmd.AddCommand(bookmarksCmd)
	rootCmd.AddCommand(accountCmd)
	rootCmd.AddCommand(autostartCmd)
	rootCmd.AddCommand(cacheCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(monitorsCmd)
	rootCmd.AddCommand(statusCmd)

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
