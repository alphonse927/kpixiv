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
	Mode                WallpaperMode            `yaml:"mode"`
	QueueSource         QueueSource              `yaml:"queue_source"`
	Orientation         WallpaperOrientation     `yaml:"orientation"`
	RotationEnabled     bool                     `yaml:"rotation_enabled"`
	FetchEnabled        bool                     `yaml:"fetch_enabled"`
	HistoryLimit        int                      `yaml:"history_limit"`
	SetInterval         int                      `yaml:"set_interval"`
	FetchInterval       int                      `yaml:"fetch_interval"`
	CleanupDays         int                      `yaml:"cleanup_days"`
	MultiMonitorEnabled bool                     `yaml:"multi_monitor_enabled"`
	Monitors            map[string]MonitorConfig `yaml:"monitors,omitempty"`
}

type WallpaperOrientation string

const (
	WallpaperAnyOrientation       WallpaperOrientation = "any"
	WallpaperLandscapeOrientation WallpaperOrientation = "landscape"
	WallpaperPortraitOrientation  WallpaperOrientation = "portrait"
)

func (o WallpaperOrientation) String() string {
	if o == "" {
		return string(WallpaperAnyOrientation)
	}
	return string(o)
}

// Label returns a human-readable orientation name for UI elements.
func (o WallpaperOrientation) Label() string {
	switch o {
	case WallpaperLandscapeOrientation:
		return "Landscape"
	case WallpaperPortraitOrientation:
		return "Portrait"
	default:
		return "Any"
	}
}

// Matches reports whether image dimensions fit this orientation. Any (or
// unknown) dimensions always match.
func (o WallpaperOrientation) Matches(width, height int) bool {
	switch o {
	case WallpaperLandscapeOrientation:
		return width >= height
	case WallpaperPortraitOrientation:
		return height > width
	default:
		return true
	}
}

// MonitorConfig controls rotation for a configured Plasma screen. An absent
// entry inherits the global wallpaper settings.
type MonitorConfig struct {
	RotationEnabled bool                 `yaml:"rotation_enabled"`
	Orientation     WallpaperOrientation `yaml:"orientation"`
}
