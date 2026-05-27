package config

type WallpaperMode int

const (
	WallpaperFillMode WallpaperMode = iota
	WallpaperCoverMode
	WallpaperFitMode
)

func (w WallpaperMode) String() string {
	switch w {
	case WallpaperFillMode:
		return "fill"
	case WallpaperCoverMode:
		return "cover"
	case WallpaperFitMode:
		return "fit"
	default:
		return "unknown"
	}
}

type WallpaperConfig struct {
	Mode          WallpaperMode `yaml:"mode"`
	HistoryLimit  int           `yaml:"history_limit"`
	SetInterval   int           `yaml:"set_interval"`
	FetchInterval int           `yaml:"fetch_interval"`
	CleanupDays   int           `yaml:"cleanup_days"`
}
