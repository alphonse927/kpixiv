package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ConfigPath string `yaml:"-"` // set during Load, not serialized

	DownloadPath  string              `yaml:"download_path"`
	Pixiv         PixivConfig         `yaml:"pixiv"`
	Bookmarks     BookmarksConfig     `yaml:"bookmarks"`
	LogLevel      string              `yaml:"log_level"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Wallpaper     WallpaperConfig     `yaml:"wallpaper"`
	KDE           KDEConfig           `yaml:"kde"`
}

type NotificationsConfig struct {
	Enabled bool `yaml:"enabled"`
}

type BookmarksConfig struct {
	Enabled      bool `yaml:"enabled"`
	SyncInterval int  `yaml:"sync_interval"`
	AutoCleanup  bool `yaml:"auto_cleanup"`
}

type KDEConfig struct {
	SetLockScreen bool `yaml:"set_lock_screen"`
}

type PixivConfig struct {
	MinWidth  int         `yaml:"min_width"`
	MinHeight int         `yaml:"min_height"`
	Ranking   RankingMode `yaml:"ranking"`
	R18       bool        `yaml:"r18"`
}

// Load reads configuration from disk or creates defaults when missing.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
		if path == "" {
			return nil, fmt.Errorf("cannot determine home directory")
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			cfg.ConfigPath = path
			if saveErr := Save(cfg.ConfigPath, cfg); saveErr != nil {
				return nil, fmt.Errorf("could not create default configuration at %s: %w", path, saveErr)
			}

			return cfg, nil
		}

		return nil, fmt.Errorf("could not read configuration at %s: %w", path, err)
	}

	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse configuration at %s: %w", path, err)
	}

	cfg.ConfigPath = path
	return &cfg, nil
}

// Save writes configuration to disk.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("could not create config directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("could not encode configuration: %w", err)
	}

	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("could not write configuration to %s: %w", path, err)
	}

	return nil
}

// Validate normalizes the configuration and enforces minimum values.
// It returns a list of human-readable adjustments that were applied,
// so callers can surface them to the user.
func (c *Config) Validate() []string {
	var issues []string

	issue := func(msg string) {
		issues = append(issues, msg)
	}

	if c.Wallpaper.Monitors == nil {
		c.Wallpaper.Monitors = map[string]MonitorConfig{}
	}

	c.normalizeOrientations(issue)
	if c.DownloadPath == "" {
		c.DownloadPath = resolveDefaultDownloadPath()
		issue(fmt.Sprintf("Download path is empty; using %s.", c.DownloadPath))
	}

	if c.Pixiv.MinWidth < DefaultMinWidth {
		c.Pixiv.MinWidth = DefaultMinWidth
		issue(fmt.Sprintf("Minimum image width is below %d; using %d.", DefaultMinWidth, DefaultMinWidth))
	}

	if c.Pixiv.MinHeight < DefaultMinHeight {
		c.Pixiv.MinHeight = DefaultMinHeight
		issue(fmt.Sprintf("Minimum image height is below %d; using %d.", DefaultMinHeight, DefaultMinHeight))
	}

	if c.Wallpaper.SetInterval < DefaultSetInterval {
		c.Wallpaper.SetInterval = DefaultSetInterval
		issue(fmt.Sprintf("Wallpaper change interval is below %d minutes; using %d.", DefaultSetInterval, DefaultSetInterval))
	}

	if c.Wallpaper.FetchInterval < DefaultFetchInterval {
		c.Wallpaper.FetchInterval = DefaultFetchInterval
		issue(fmt.Sprintf("Fetch interval is below %d minutes; using %d.", DefaultFetchInterval, DefaultFetchInterval))
	}

	if c.Wallpaper.HistoryLimit < 1 {
		c.Wallpaper.HistoryLimit = DefaultHistoryLimit
		issue(fmt.Sprintf("History limit is invalid; using %d.", DefaultHistoryLimit))
	}

	if c.Wallpaper.CleanupDays < 1 {
		c.Wallpaper.CleanupDays = DefaultCleanupDays
		issue(fmt.Sprintf("Cleanup age is below 1 day; using %d days.", DefaultCleanupDays))
	}

	if c.Bookmarks.SyncInterval < DefaultBookmarksSync {
		c.Bookmarks.SyncInterval = DefaultBookmarksSync
		issue(fmt.Sprintf("Bookmark sync interval is below %d minutes; using %d.", DefaultBookmarksSync, DefaultBookmarksSync))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		c.LogLevel = DefaultLogLevel
		issue(fmt.Sprintf("Unknown log level; using %q.", DefaultLogLevel))
	}

	return issues
}

func (c *Config) normalizeOrientations(issue func(string)) {
	switch c.Wallpaper.Orientation {
	case WallpaperLandscapeOrientation, WallpaperPortraitOrientation, WallpaperAnyOrientation, "":
	default:
		c.Wallpaper.Orientation = WallpaperAnyOrientation
		issue("Wallpaper orientation is unknown; using \"any\".")
	}

	for id, monitor := range c.Wallpaper.Monitors {
		switch monitor.Orientation {
		case WallpaperLandscapeOrientation, WallpaperPortraitOrientation, WallpaperAnyOrientation, "":
		default:
			monitor.Orientation = WallpaperAnyOrientation
			c.Wallpaper.Monitors[id] = monitor
			issue(fmt.Sprintf("Monitor %s has an unknown orientation; using \"any\".", id))
		}
	}
}

func resolveDefaultDownloadPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return DefaultDownloadPath
	}

	return filepath.Join(homeDir, DefaultDownloadPath)
}
