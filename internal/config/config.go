package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DownloadPath string          `yaml:"download_path"`
	Pixiv        PixivConfig     `yaml:"pixiv"`
	Wallpaper    WallpaperConfig `yaml:"wallpaper"`
	KDE          KDEConfig       `yaml:"kde"`
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
	DefaultDownloadPath  = "~/Pictures/KPixiv"
	DefaultPixivRanking  = 0
	DefaultMinWidth      = 1280
	DefaultMinHeight     = 720
	DefaultHistoryLimit  = 10
	DefaultSetInterval   = 5
	DefaultFetchInterval = 30
	DefaultCleanupDays   = 7
)

func Default() *Config {
	return &Config{
		DownloadPath: DefaultDownloadPath,
		Pixiv: PixivConfig{
			Ranking:       DefaultPixivRanking,
			R18:           false,
			MinWidth:      DefaultMinWidth,
			MinHeight:     DefaultMinHeight,
			LandscapeOnly: true,
		},
		Wallpaper: WallpaperConfig{
			Mode:          WallpaperFillMode,
			HistoryLimit:  DefaultHistoryLimit,
			SetInterval:   DefaultSetInterval,
			FetchInterval: DefaultFetchInterval,
			CleanupDays:   DefaultCleanupDays,
		},
		KDE: KDEConfig{
			SetLockScreen: false,
		},
	}
}

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
			if saveErr := Save(path, cfg); saveErr != nil {
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

	return &cfg, nil
}

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

func (c *Config) Validate() error {
	if c.DownloadPath == "" {
		c.DownloadPath = DefaultDownloadPath
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
	return nil
}
