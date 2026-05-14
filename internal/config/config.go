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
	MinWidth      int         `yaml:"min_width"`
	MinHeight     int         `yaml:"min_height"`
	Ranking       RankingMode `yaml:"ranking"`
	R18           bool        `yaml:"r18"`
	LandscapeOnly bool        `yaml:"landscape_only"`
}

const (
	DefaultIntervalMinutes = 30
	DefaultDownloadPath    = "~/Pictures/KPixiv"
	DefaultPixivRanking    = 0
	DefaultMinWidth        = 1280
	DefaultMinHeight       = 720
	DefaultKeepHistory     = 5
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
			Mode:        WallpaperFillMode,
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
		c.IntervalMinutes = DefaultIntervalMinutes
	}
	if c.DownloadPath == "" {
		c.DownloadPath = DefaultDownloadPath
	}
	if c.Pixiv.MinWidth < 1280 {
		c.Pixiv.MinWidth = DefaultMinWidth
	}
	if c.Pixiv.MinHeight < 720 {
		c.Pixiv.MinHeight = DefaultMinHeight
	}
	if c.Wallpaper.KeepHistory < 5 {
		c.Wallpaper.KeepHistory = DefaultKeepHistory
	}
	return nil
}
