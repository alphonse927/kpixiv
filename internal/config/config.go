package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ConfigPath string `yaml:"-"` // set during Load, not serialized

	DownloadPath string          `yaml:"download_path"`
	Pixiv        PixivConfig     `yaml:"pixiv"`
	Bookmarks    BookmarksConfig `yaml:"bookmarks"`
	Wallpaper    WallpaperConfig `yaml:"wallpaper"`
	KDE          KDEConfig       `yaml:"kde"`
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
	MinWidth      int         `yaml:"min_width"`
	MinHeight     int         `yaml:"min_height"`
	Ranking       RankingMode `yaml:"ranking"`
	R18           bool        `yaml:"r18"`
	LandscapeOnly bool        `yaml:"landscape_only"`
}

const (
	DefaultDownloadPath     = "Pictures/KPixiv"
	DefaultPixivRanking     = 0
	DefaultMinWidth         = 1280
	DefaultMinHeight        = 720
	DefaultHistoryLimit     = 10
	DefaultSetInterval      = 5
	DefaultFetchInterval    = 30
	DefaultCleanupDays      = 7
	DefaultBookmarksSync    = 60
	DefaultBookmarksEnabled = false
	DefaultBookmarksCleanup = true
)

// Default returns the default application configuration.
func Default() *Config {
	defaultDownloadPath := resolveDefaultDownloadPath()

	return &Config{
		DownloadPath: defaultDownloadPath,
		Pixiv: PixivConfig{
			Ranking:       DefaultPixivRanking,
			R18:           false,
			MinWidth:      DefaultMinWidth,
			MinHeight:     DefaultMinHeight,
			LandscapeOnly: true,
		},
		Wallpaper: WallpaperConfig{
			Mode:            WallpaperFillMode,
			QueueSource:     QueueSourceAll,
			RotationEnabled: true,
			FetchEnabled:    true,
			HistoryLimit:    DefaultHistoryLimit,
			SetInterval:     DefaultSetInterval,
			FetchInterval:   DefaultFetchInterval,
			CleanupDays:     DefaultCleanupDays,
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

// Load reads configuration from disk or creates defaults when missing.
func Load(path string) (*Config, error) {
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(homeDir, ".config", "kpixiv", "config.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			cfg.ConfigPath = path
			if saveErr := Save(cfg.ConfigPath, cfg); saveErr != nil {
				return nil, saveErr
			}
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	cfg.ConfigPath = path
	return &cfg, nil
}

// Save writes configuration to disk.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// Validate normalizes and enforces minimum configuration values.
func (c *Config) Validate() {
	if c.DownloadPath == "" {
		c.DownloadPath = resolveDefaultDownloadPath()
	}
	if c.Pixiv.MinWidth < 1280 {
		c.Pixiv.MinWidth = DefaultMinWidth
	}
	if c.Pixiv.MinHeight < 720 {
		c.Pixiv.MinHeight = DefaultMinHeight
	}
	if c.Wallpaper.SetInterval < 5 {
		c.Wallpaper.SetInterval = DefaultSetInterval
	}
	if c.Wallpaper.FetchInterval < 30 {
		c.Wallpaper.FetchInterval = DefaultFetchInterval
	}
	if c.Wallpaper.HistoryLimit < 1 {
		c.Wallpaper.HistoryLimit = DefaultHistoryLimit
	}
	if c.Wallpaper.CleanupDays < 1 {
		c.Wallpaper.CleanupDays = DefaultCleanupDays
	}
	if c.Bookmarks.SyncInterval < 60 {
		c.Bookmarks.SyncInterval = DefaultBookmarksSync
	}
}

func resolveDefaultDownloadPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return DefaultDownloadPath
	}

	return filepath.Join(homeDir, DefaultDownloadPath)
}
