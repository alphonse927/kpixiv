package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	IntervalMinutes int             `yaml:"interval_minutes"`
	DownloadPath    string          `yaml:"download_path"`
	Pixiv           PixivConfig     `yaml:"pixiv"`
	Wallpaper       WallpaperConfig `yaml:"wallpaper"`
}

type PixivConfig struct {
	Ranking       string `yaml:"ranking"`
	R18           bool   `yaml:"r18"`
	MinWidth      int    `yaml:"min_width"`
	MinHeight     int    `yaml:"min_height"`
	LandscapeOnly bool   `yaml:"landscape_only"`
}

type WallpaperConfig struct {
	Mode        string `yaml:"mode"`
	KeepHistory int    `yaml:"keep_history"`
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
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
