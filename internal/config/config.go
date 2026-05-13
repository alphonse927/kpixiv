package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	IntervalMinutes int             `toml:"interval_minutes"`
	DownloadPath    string          `toml:"download_path"`
	Pixiv           PixivConfig     `toml:"pixiv"`
	Wallpaper       WallpaperConfig `toml:"wallpaper"`
}

type PixivConfig struct {
	Ranking       string `toml:"ranking"`
	R18           bool   `toml:"r18"`
	MinWidth      int    `toml:"min_width"`
	MinHeight     int    `toml:"min_height"`
	LandscapeOnly bool   `toml:"landscape_only"`
}

type WallpaperConfig struct {
	Mode        string `toml:"mode"`
	KeepHistory int    `toml:"keep_history"`
}

const (
	DefaultIntervalMinutes = 30
	DefaultDownloadPath    = "~/Pictures/KPixiv"
	DefaultPixivRanking    = "daily"
	DefaultMinWidth        = 1920
	DefaultMinHeight       = 1080
	DefaultKeepHistory     = 20
)

func Default() *Config {
	return &Config{
		IntervalMinutes: DefaultIntervalMinutes,
		DownloadPath:    DefaultDownloadPath,
		Pixiv: PixivConfig{
			Ranking:       DefaultPixivRanking,
			R18:           false,
			MinWidth:      DefaultMinWidth,
			MinHeight:     DefaultMinHeight,
			LandscapeOnly: true,
		},
		Wallpaper: WallpaperConfig{
			Mode:        "fill",
			KeepHistory: DefaultKeepHistory,
		},
	}
}

func Load(path string) (*Config, error) {
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(homeDir, ".config", "kpixiv", "config.toml")
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
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (c *Config) Validate() error {
	if c.IntervalMinutes < 5 {
		c.IntervalMinutes = 5
	}
	if c.DownloadPath == "" {
		c.DownloadPath = DefaultDownloadPath
	}
	if c.Pixiv.MinWidth < 800 {
		c.Pixiv.MinWidth = 800
	}
	if c.Pixiv.MinHeight < 600 {
		c.Pixiv.MinHeight = 600
	}
	if c.Wallpaper.KeepHistory < 5 {
		c.Wallpaper.KeepHistory = 5
	}
	return nil
}
