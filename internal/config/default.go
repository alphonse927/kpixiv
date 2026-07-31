package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultDownloadPath         = "Pictures/KPixiv"
	DefaultPixivRanking         = 0
	DefaultMinWidth             = 1280
	DefaultMinHeight            = 720
	DefaultHistoryLimit         = 10
	DefaultSetInterval          = 5
	DefaultFetchInterval        = 30
	DefaultCleanupDays          = 7
	DefaultBookmarksSync        = 60
	DefaultBookmarksEnabled     = false
	DefaultBookmarksCleanup     = true
	DefaultLogLevel             = "info"
	DefaultNotificationsEnabled = true
)

// Default returns the default application configuration.
func Default() *Config {
	defaultDownloadPath := resolveDefaultDownloadPath()

	return &Config{
		DownloadPath: defaultDownloadPath,
		Pixiv: PixivConfig{
			Ranking:   DefaultPixivRanking,
			R18:       false,
			MinWidth:  DefaultMinWidth,
			MinHeight: DefaultMinHeight,
		},
		Wallpaper: WallpaperConfig{
			Mode:                WallpaperFillMode,
			QueueSource:         QueueSourceAll,
			Orientation:         WallpaperAnyOrientation,
			RotationEnabled:     true,
			FetchEnabled:        true,
			HistoryLimit:        DefaultHistoryLimit,
			SetInterval:         DefaultSetInterval,
			FetchInterval:       DefaultFetchInterval,
			CleanupDays:         DefaultCleanupDays,
			MultiMonitorEnabled: false,
			Monitors:            map[string]MonitorConfig{},
		},
		LogLevel: DefaultLogLevel,
		Notifications: NotificationsConfig{
			Enabled: DefaultNotificationsEnabled,
		},
		Bookmarks: BookmarksConfig{
			Enabled:      DefaultBookmarksEnabled,
			SyncInterval: DefaultBookmarksSync,
			AutoCleanup:  DefaultBookmarksCleanup,
		},
		KDE: KDEConfig{
			SetLockScreen: false,
		},
	}
}

// DefaultPath returns the default configuration file location.
func DefaultPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".config", "kpixiv", "config.yaml")
}
