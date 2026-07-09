package config

type WallpaperMode int

const (
	WallpaperFillMode WallpaperMode = iota
	WallpaperCoverMode
	WallpaperFitMode
)

// String returns the string representation of a wallpaper mode.
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
	Mode            WallpaperMode `yaml:"mode"`
	QueueSource     QueueSource   `yaml:"queue_source"`
	RotationEnabled bool          `yaml:"rotation_enabled"`
	FetchEnabled    bool          `yaml:"fetch_enabled"`
	HistoryLimit    int           `yaml:"history_limit"`
	SetInterval     int           `yaml:"set_interval"`
	FetchInterval   int           `yaml:"fetch_interval"`
	CleanupDays     int           `yaml:"cleanup_days"`
}
