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
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("could not load configuration:\n  %w", err)
		}

		cfg.Validate()

		logger.Init(verbose)
		if !verbose {
			logger.SetLevel(cfg.LogLevel)
		}
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version and build information",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil // version must work even with a broken config
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("kpixivctl %s\n", build.Version)
		if build.Commit != "" {
			fmt.Printf("Commit:        %s\n", build.Commit)
		}
		if build.Date != "" {
			fmt.Printf("Build date:    %s\n", build.Date)
		}
		fmt.Printf("Go version:    %s\n", build.GoVersion)
		if fyneVersion := build.FyneVersion(); fyneVersion != "" {
			fmt.Printf("Fyne version:  %s\n", fyneVersion)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		ctx := context.Background()
		f := fetcher.NewFetcher(cfg, st, pixivClient)
		if err = f.LoadPage(); err != nil {
			return fmt.Errorf("could not load ranking page:\n  %w", err)
		}

		if dryRun {
			if _, err = f.DryRun(ctx); err != nil {
				return fmt.Errorf("fetch failed:\n  %w", err)
			}
			return nil
		}

		result, err := f.Fetch(ctx)
		if err != nil {
			return fmt.Errorf("fetch failed:\n  %w", err)
		}

		fmt.Printf("Fetched from %s ranking.\n", capitalize(cfg.Pixiv.Ranking.String()))
		fmt.Printf("Total: %d, Downloaded: %d, Filtered: %d, Skipped: %d, Failed: %d\n",
			result.Total, result.Downloaded, result.Filtered, result.Skipped, result.Failed)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		var setter wallpaper.Setter
		if dryRun {
			setter = wallpaper.NewDryRunSetter()
		} else {
			setter = wallpaper.NewKDESetter(cfg.KDE.SetLockScreen)
		}

		q := storage.NewQueue(s.StateDir())
		if err := q.Load(); err != nil {
			return fmt.Errorf("could not load queue:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err = q.Load(); err != nil {
			return fmt.Errorf("could not load queue:\n  %w", err)
		}

		if err = q.Clear(); err != nil {
			return fmt.Errorf("could not clear queue:\n  %w", err)
		}

		blacklist, err := st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("could not load blacklist:\n  %w", err)
		}

		ids := scanDirForImages(st.BookmarksDir(), blacklist)
		if len(ids) == 0 {
			fmt.Println("No images found in Bookmarks folder")
			return nil
		}

		if err := st.AddBookmarks(ids); err != nil {
			return fmt.Errorf("could not update bookmarks:\n  %w", err)
		}

		if err := q.AppendRandom(ids); err != nil {
			return fmt.Errorf("could not populate queue:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err = q.Load(); err != nil {
			return fmt.Errorf("could not load queue:\n  %w", err)
		}

		if err = q.Clear(); err != nil {
			return fmt.Errorf("could not clear queue:\n  %w", err)
		}

		blacklist, err := st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("could not load blacklist:\n  %w", err)
		}

		ids := scanDirForImages(st.RankingDir(), blacklist)
		if len(ids) == 0 {
			fmt.Println("No images found in Ranking folder")
			return nil
		}

		if err := q.AppendRandom(ids); err != nil {
			return fmt.Errorf("could not populate queue:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		if err = q.Load(); err != nil {
			return fmt.Errorf("could not load queue:\n  %w", err)
		}

		if err = q.Clear(); err != nil {
			return fmt.Errorf("could not clear queue:\n  %w", err)
		}

		blacklist, err := st.LoadBlacklistSet()
		if err != nil {
			return fmt.Errorf("could not load blacklist:\n  %w", err)
		}

		rankingIDs := scanDirForImages(st.RankingDir(), blacklist)
		bookmarksIDs := scanDirForImages(st.BookmarksDir(), blacklist)

		if len(bookmarksIDs) > 0 {
			if err := st.AddBookmarks(bookmarksIDs); err != nil {
				return fmt.Errorf("could not update bookmarks:\n  %w", err)
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
			return fmt.Errorf("could not populate queue:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		if !pixivClient.LoggedIn() {
			return fmt.Errorf("you must be logged in to sync your bookmarks — run 'kpixivctl account login'")
		}

		ctx := context.Background()
		syncer := bookmarks.NewSyncer(cfg, st, pixivClient)
		result, err := syncer.Sync(ctx)
		if err != nil {
			if errors.Is(err, pixiv.ErrAuthSessionInvalid) {
				return fmt.Errorf("pixiv login has expired — run 'kpixivctl account login' to reconnect:\n  %w", err)
			}
			return fmt.Errorf("could not sync bookmarks:\n  %w", err)
		}

		fmt.Printf("Bookmark sync complete!\n")
		fmt.Printf("Total: %d, Filtered: %d, Downloaded: %d, Deleted: %d, Skipped: %d, Failed: %d\n",
			result.Total, result.Filtered, result.Downloaded, result.Deleted, result.Skipped, result.Failed)
		return nil
	},
}

var bookmarksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List locally bookmarked images",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := storage.New("", cfg.DownloadPath)
		if err != nil {
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		bookmarkIDs, err := st.LoadBookmarks()
		if err != nil {
			return fmt.Errorf("could not load bookmarks:\n  %w", err)
		}

		metadata, err := st.LoadMetadata()
		if err != nil {
			return fmt.Errorf("could not load metadata:\n  %w", err)
		}

		if len(bookmarkIDs) == 0 {
			fmt.Println("No bookmarks found.")
			return nil
		}

		fmt.Println("Bookmarked Images")
		fmt.Println("─────────────────")
		for id := range bookmarkIDs {
			if meta, ok := metadata[id]; ok {
				title := meta.Title
				if title == "" {
					title = "(untitled)"
				}
				artist := meta.Artist
				if artist == "" {
					artist = "(unknown artist)"
				}
				fmt.Printf("  %s  %s by %s [%dx%d]\n", id, title, artist, meta.Width, meta.Height)
			} else {
				fmt.Printf("  %s  (not downloaded)\n", id)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		if !pixivClient.LoggedIn() {
			return fmt.Errorf("you must be logged in to bookmark artwork — run 'kpixivctl account login'")
		}

		ctx := context.Background()
		if err := pixivClient.BookmarkIllust(ctx, illustID); err != nil {
			return fmt.Errorf("could not bookmark on pixiv:\n  %w", err)
		}

		if err := st.AddBookmark(illustID); err != nil {
			return fmt.Errorf("could not save local bookmark:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		if !pixivClient.LoggedIn() {
			return fmt.Errorf("you must be logged in to bookmark artwork — run 'kpixivctl account login'")
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
				return fmt.Errorf("could not load monitor history:\n  %w", err)
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
				return fmt.Errorf("could not list monitors:\n  %w", err)
			}

			monitors, err := st.LoadMonitorHistory()
			if err != nil {
				return fmt.Errorf("could not load monitor history:\n  %w", err)
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
				return fmt.Errorf("could not get current wallpaper:\n  %w", err)
			}
			if currentID == "" {
				return fmt.Errorf("no current wallpaper")
			}
			currentIDs = []string{currentID}
		}

		for _, id := range currentIDs {
			if err := pixivClient.BookmarkIllust(ctx, id); err != nil {
				return fmt.Errorf("could not bookmark artwork %s on pixiv:\n  %w", id, err)
			}

			if err := st.AddBookmark(id); err != nil {
				return fmt.Errorf("could not save local bookmark for %s:\n  %w", id, err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		logger.Info("Opening browser for Pixiv authentication...")
		_, err = auth.Login(context.Background(), auth.LoginConfig{}, &pixiv.AuthProvider{Client: pixivClient})
		if err != nil {
			return fmt.Errorf("login failed:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		if err := pixivClient.Logout(); err != nil {
			return fmt.Errorf("could not log out:\n  %w", err)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		pixivClient, err := pixiv.NewClient(st.StateDir())
		if err != nil {
			return fmt.Errorf("could not initialize pixiv client:\n  %w", err)
		}

		if pixivClient.LoggedIn() {
			userName := pixivClient.AuthUserName()
			if userName != "" {
				fmt.Printf("Logged in as: %s\n", userName)
			} else {
				fmt.Println("Logged in to Pixiv")
			}

			if expiry := pixivClient.AuthExpiry(); !expiry.IsZero() {
				fmt.Printf("Token valid until: %s\n", expiry.Format(time.DateTime))
			}
		} else {
			fmt.Println("Not logged in")
			fmt.Println("Run 'kpixivctl account login' to connect your Pixiv account.")
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		stats, err := st.CacheStats()
		if err != nil {
			return fmt.Errorf("could not read cache statistics:\n  %w", err)
		}

		fmt.Printf("Downloaded images: %d\n", stats.Count)
		if stats.Size > 0 {
			fmt.Printf("Disk usage:        %s\n", formatBytes(stats.Size))
		}
		if !stats.Oldest.IsZero() {
			fmt.Printf("Oldest image:      %s\n", stats.Oldest.Format(time.DateTime))
		}
		if !stats.Newest.IsZero() {
			fmt.Printf("Newest image:      %s\n", stats.Newest.Format(time.DateTime))
		}

		q := storage.NewQueue(st.StateDir())
		if err := q.Load(); err == nil {
			fmt.Printf("In queue:          %d\n", q.Len())
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		result, err := st.CleanupImagesOlderThanDays(0)
		if err != nil {
			return fmt.Errorf("could not clear cache:\n  %w", err)
		}

		fmt.Printf("Removed %d images", result.Removed)
		if result.FreedBytes > 0 {
			fmt.Printf(" (%s freed)", formatBytes(result.FreedBytes))
		}
		fmt.Println()
		return nil
	},
}

var autostartCmd = &cobra.Command{
	Use:   "autostart",
	Short: "Manage systemd autostart",
}

var autostartEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Install, enable, and start the systemd user service",
	Long: `Installs the systemd unit file, enables the KPixiv user service for
automatic startup on login, and starts it right now.

The unit file is written to ~/.config/systemd/user/kpixiv.service.
This is the only supported way to run kPixiv persistently -- see
` + "`kpixiv --help`" + ` for the advanced --foreground debug mode.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := platform.EnableService("kpixiv.service"); err != nil {
			return err
		}
		fmt.Println("KPixiv autostart enabled and started.")
		return nil
	},
}

var autostartDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Stop kPixiv and disable automatic startup",
	Long: `Stops kPixiv right now and prevents it from starting automatically
when you log in. The service unit file is kept on disk so the
service state remains queryable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := platform.DisableService("kpixiv.service"); err != nil {
			return err
		}
		fmt.Println("KPixiv stopped and autostart disabled.")
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
			return fmt.Errorf("could not read configuration file:\n  %w", err)
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
  notifications.enabled       (bool)
  pixiv.r18                   (bool)
  pixiv.ranking               (int: 0=daily, 1=weekly, 2=monthly)
  pixiv.min_width             (int, minimum 1280)
  pixiv.min_height            (int, minimum 720)
  wallpaper.orientation       (string: any/landscape/portrait)
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
		if err := cfg.Set(args[0], args[1]); err != nil {
			return err
		}

		issues := cfg.Validate()
		if err := config.Save(cfg.ConfigPath, cfg); err != nil {
			return fmt.Errorf("could not save configuration:\n  %w", err)
		}

		fmt.Printf("Set %s = %s\n", args[0], args[1])
		for _, issue := range issues {
			fmt.Printf("Note: %s\n", issue)
		}
		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset the configuration to default values",
	Long: `Resets the configuration file to the default values.

This overwrites the current configuration, restoring stock defaults for every
setting. Custom values such as the download directory and log level are
forgotten.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil // reset must work even with a broken config file
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgPath
		if path == "" {
			path = config.DefaultPath()
			if path == "" {
				return fmt.Errorf("cannot determine home directory")
			}
		}

		reset := config.Default()
		reset.ConfigPath = path
		if err := config.Save(path, reset); err != nil {
			return fmt.Errorf("could not reset configuration:\n  %w", err)
		}

		fmt.Printf("Configuration reset to defaults: %s\n", path)
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
			return fmt.Errorf("could not query Plasma screens:\n  %w", err)
		}

		if len(screens) == 0 {
			fmt.Println("No active Plasma screens found")
			return nil
		}

		fmt.Println("Active Plasma Screens")
		fmt.Println("─────────────────────")
		for _, screen := range screens {
			state := "enabled"
			if settings, ok := cfg.Wallpaper.Monitors[screen.ID]; ok && !settings.RotationEnabled {
				state = "disabled"
			}

			name := screen.Name
			if name == "" {
				name = "Screen " + screen.ID
			}

			orientation := config.WallpaperAnyOrientation.String()
			if settings, ok := cfg.Wallpaper.Monitors[screen.ID]; ok && settings.Orientation != "" {
				orientation = string(settings.Orientation)
			}

			if screen.Model != "" {
				fmt.Printf("  [%s] %s (%s/%s)\trotation=%s\torientation=%s\n",
					screen.Index, name, screen.Model, screen.ID, state, orientation)
			} else {
				fmt.Printf("  [%s] %s (%s)\trotation=%s\torientation=%s\n",
					screen.Index, name, screen.ID, state, orientation)
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
			return fmt.Errorf("could not initialize storage:\n  %w", err)
		}

		history, err := st.LoadHistory()
		if err != nil {
			return fmt.Errorf("could not load history:\n  %w", err)
		}
		activity, err := st.LoadActivity()
		if err != nil {
			return fmt.Errorf("could not load activity:\n  %w", err)
		}
		monitorHistory, monitorHistoryErr := st.LoadMonitorHistory()

		stats, err := st.CacheStats()
		if err != nil {
			return fmt.Errorf("could not read cache statistics:\n  %w", err)
		}

		q := storage.NewQueue(st.StateDir())
		queueLen := 0
		nextID := ""
		if qErr := q.Load(); qErr == nil {
			queueLen = q.Len()
			if id, ok := q.Peek(); ok {
				nextID = id
			}
		}

		displayPath := func(p string) string {
			home, _ := os.UserHomeDir() //nolint:errcheck // cosmetic display only
			if home != "" && strings.HasPrefix(p, home) {
				return "~" + strings.TrimPrefix(p, home)
			}
			return p
		}

		fmt.Println("KPixiv Status")
		fmt.Println("─────────────")

		// Configuration.
		width := 22
		fmt.Println()
		fmt.Println("Configuration")
		fmt.Printf("%s\n", strings.Repeat("─", 22))
		fmt.Println(keyValue(width, "Version", build.Version))
		fmt.Println(keyValue(width, "Config file", displayPath(cfg.ConfigPath)))
		fmt.Println(keyValue(width, "Download dir", displayPath(st.DownloadDir())))
		fmt.Println(keyValue(width, "Data dir", displayPath(st.DataDir())))
		fmt.Println(keyValue(width, "State dir", displayPath(st.StateDir())))

		// Wallpaper settings.
		fmt.Println()
		fmt.Println("Wallpaper")
		fmt.Println("─────────")
		fmt.Println(keyValue(width, "Feed source", feedSourceLabel(cfg)))
		fmt.Println(keyValue(width, "Orientation", cfg.Wallpaper.Orientation.String()))
		fmt.Println(keyValue(width, "Rotation", boolLabel(cfg.Wallpaper.RotationEnabled)))
		fmt.Println(keyValue(width, "Change interval", fmt.Sprintf("%d minutes", cfg.Wallpaper.SetInterval)))
		fmt.Println(keyValue(width, "Fetch interval", fmt.Sprintf("%d minutes", cfg.Wallpaper.FetchInterval)))
		fmt.Println(keyValue(width, "Cleanup age", fmt.Sprintf("%d days", cfg.Wallpaper.CleanupDays)))
		fmt.Println(keyValue(width, "Multi-monitor", boolLabel(cfg.Wallpaper.MultiMonitorEnabled)))
		fmt.Println(keyValue(width, "Lock screen", boolLabel(cfg.KDE.SetLockScreen)))

		// Current state.
		fmt.Println()
		fmt.Println("Current State")
		fmt.Println("─────────────")
		fmt.Println(keyValue(width, "Daemon", platformStateLabel()))
		fmt.Println(keyValue(width, "Pixiv", authStateLabel(st)))
		fmt.Println(keyValue(width, "Current wallpaper", orNone(history.Current)))
		fmt.Println(keyValue(width, "Next wallpaper", orNone(nextID)))
		fmt.Println(keyValue(width, "Last change", lastChangeLabel(history.UpdatedAt)))
		fmt.Println(keyValue(width, "Next change", nextChangeLabel(cfg, history.UpdatedAt)))
		fmt.Println(keyValue(width, "Last fetch", lastActivityLabel(activity.LastFetchAt)))
		fmt.Println(keyValue(width, "Next fetch", nextFetchLabel(cfg, activity.LastFetchAt)))
		if cfg.Bookmarks.Enabled {
			fmt.Println(keyValue(width, "Last bookmark sync", lastActivityLabel(activity.LastBookmarkSyncAt)))
			fmt.Println(keyValue(width, "Next bookmark sync", nextBookmarkSyncLabel(cfg, activity.LastBookmarkSyncAt)))
		}

		// Cache.
		fmt.Println()
		fmt.Println("Cache")
		fmt.Println("─────")
		fmt.Println(keyValue(width, "Wallpapers", cacheCountLabel(stats)))
		fmt.Println(keyValue(width, "Disk usage", formatBytes(stats.Size)))
		fmt.Println(keyValue(width, "In queue", queueLen))
		fmt.Println(keyValue(width, "History entries", historyEntriesLabel(history)))
		if !stats.Oldest.IsZero() {
			fmt.Println(keyValue(width, "Oldest image", stats.Oldest.Format(time.DateTime)))
		}
		if !stats.Newest.IsZero() {
			fmt.Println(keyValue(width, "Newest image", stats.Newest.Format(time.DateTime)))
		}

		// Monitors.
		screens, err := wallpaper.NewKDESetter(false).Screens()
		if err != nil {
			screens = nil
		}
		if len(screens) > 0 {
			fmt.Println()
			fmt.Println("Monitors")
			fmt.Println("────────")
			for _, s := range screens {
				model := s.Model
				if model == "" {
					model = "(unknown)"
				}
				rotation := "enabled"
				if settings, configured := cfg.Wallpaper.Monitors[s.ID]; configured && !settings.RotationEnabled {
					rotation = "disabled"
				}
				orientation := config.WallpaperAnyOrientation.String()
				if settings, configured := cfg.Wallpaper.Monitors[s.ID]; configured && settings.Orientation != "" {
					orientation = string(settings.Orientation)
				}

				current := ""
				if monitorHistoryErr == nil {
					current = monitorHistory[s.ID]
				}

				fmt.Printf("  [%s] %s (%s)\trotation=%s\torientation=%s\tcurrent=%s\n",
					s.Index, s.ID, model, rotation, orientation, orNone(current))
			}
		}

		log.Debug("Status displayed")
		return nil
	},
}

func feedSourceLabel(cfg *config.Config) string {
	switch cfg.Wallpaper.QueueSource {
	case config.QueueSourceBookmarks:
		return "Bookmarks"
	case config.QueueSourceAll:
		return cfg.Pixiv.Ranking.String() + " Ranking + Bookmarks"
	default:
		return cfg.Pixiv.Ranking.String() + " Ranking"
	}
}

func boolLabel(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func platformStateLabel() string {
	if platform.IsInstanceRunning() {
		return "running"
	}
	return "not running"
}

func authStateLabel(st *storage.Storage) string {
	client, err := pixiv.NewClient(st.StateDir())
	if err != nil {
		return "unknown"
	}
	if !client.LoggedIn() {
		return "not connected"
	}
	if user := client.AuthUserName(); user != "" {
		return "connected (" + user + ")"
	}
	return "connected"
}

func cacheCountLabel(stats storage.CacheStats) string {
	if stats.Count == 0 {
		return "empty"
	}
	return strconv.Itoa(stats.Count)
}

func historyEntriesLabel(h *storage.History) string {
	count := len(h.Images)
	if h.Current != "" {
		count++
	}
	return strconv.Itoa(count)
}

func lastChangeLabel(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.DateTime) + " (" + formatDuration(time.Since(t)) + " ago)"
}

func nextChangeLabel(cfg *config.Config, last time.Time) string {
	if last.IsZero() {
		return "not scheduled"
	}
	interval := time.Duration(cfg.Wallpaper.SetInterval) * time.Minute
	remaining := time.Until(last.Add(interval))
	if remaining <= 0 {
		return "any moment now"
	}
	return "in " + formatDuration(remaining)
}

func lastActivityLabel(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.DateTime) + " (" + formatDuration(time.Since(t)) + " ago)"
}

func nextFetchLabel(cfg *config.Config, last time.Time) string {
	if !cfg.Wallpaper.FetchEnabled {
		return "disabled"
	}
	if last.IsZero() {
		return "in " + formatDuration(time.Duration(cfg.Wallpaper.FetchInterval)*time.Minute)
	}
	return nextInLabel(last.Add(time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute))
}

func nextBookmarkSyncLabel(cfg *config.Config, last time.Time) string {
	if !cfg.Bookmarks.Enabled {
		return "disabled"
	}
	if last.IsZero() {
		return "in " + formatDuration(time.Duration(cfg.Bookmarks.SyncInterval)*time.Minute)
	}
	return nextInLabel(last.Add(time.Duration(cfg.Bookmarks.SyncInterval) * time.Minute))
}

func nextInLabel(next time.Time) string {
	remaining := time.Until(next)
	if remaining <= 0 {
		return "any moment now"
	}
	return "in " + formatDuration(remaining)
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
	configCmd.AddCommand(configResetCmd)

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
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(doctorCmd)
}

func main() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
