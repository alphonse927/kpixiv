package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	MinWidth      int         `yaml:"min_width"`
	MinHeight     int         `yaml:"min_height"`
	Ranking       RankingMode `yaml:"ranking"`
	R18           bool        `yaml:"r18"`
	LandscapeOnly bool        `yaml:"landscape_only"`
}

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
			Ranking:       DefaultPixivRanking,
			R18:           false,
			MinWidth:      DefaultMinWidth,
			MinHeight:     DefaultMinHeight,
			LandscapeOnly: true,
		},
		Wallpaper: WallpaperConfig{
			Mode:                WallpaperFillMode,
			QueueSource:         QueueSourceAll,
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
	for id, monitor := range c.Wallpaper.Monitors {
		switch monitor.Orientation {
		case WallpaperLandscapeOrientation, WallpaperPortraitOrientation, WallpaperAnyOrientation, "":
		default:
			monitor.Orientation = WallpaperAnyOrientation
			c.Wallpaper.Monitors[id] = monitor
			issue(fmt.Sprintf("Monitor %s has an unknown orientation; using \"any\".", id))
		}
	}
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

// valueKind describes how a config value is parsed before being stored.
type valueKind int

const (
	valueInt valueKind = iota
	valueBool
	valueEnum
)

// keySetter maps a dotted config key to its parser and storage target.
type keySetter struct {
	kind    valueKind
	setInt  func(v int)
	setBool func(v bool)
	setStr  func(v string)
	allowed []string
}

// setters returns the registry of supported configuration keys.
func (c *Config) setters() map[string]keySetter {
	return map[string]keySetter{
		"log_level": {
			kind:    valueEnum,
			setStr:  func(v string) { c.LogLevel = v },
			allowed: []string{"debug", "info", "warn", "error"},
		},
		"notifications.enabled":           {kind: valueBool, setBool: func(v bool) { c.Notifications.Enabled = v }},
		"pixiv.r18":                       {kind: valueBool, setBool: func(v bool) { c.Pixiv.R18 = v }},
		"pixiv.landscape_only":            {kind: valueBool, setBool: func(v bool) { c.Pixiv.LandscapeOnly = v }},
		"pixiv.ranking":                   {kind: valueInt, setInt: func(v int) { c.Pixiv.Ranking = RankingMode(v) }},
		"pixiv.min_width":                 {kind: valueInt, setInt: func(v int) { c.Pixiv.MinWidth = v }},
		"pixiv.min_height":                {kind: valueInt, setInt: func(v int) { c.Pixiv.MinHeight = v }},
		"wallpaper.set_interval":          {kind: valueInt, setInt: func(v int) { c.Wallpaper.SetInterval = v }},
		"wallpaper.fetch_interval":        {kind: valueInt, setInt: func(v int) { c.Wallpaper.FetchInterval = v }},
		"wallpaper.history_limit":         {kind: valueInt, setInt: func(v int) { c.Wallpaper.HistoryLimit = v }},
		"wallpaper.cleanup_days":          {kind: valueInt, setInt: func(v int) { c.Wallpaper.CleanupDays = v }},
		"wallpaper.rotation_enabled":      {kind: valueBool, setBool: func(v bool) { c.Wallpaper.RotationEnabled = v }},
		"wallpaper.fetch_enabled":         {kind: valueBool, setBool: func(v bool) { c.Wallpaper.FetchEnabled = v }},
		"wallpaper.multi_monitor_enabled": {kind: valueBool, setBool: func(v bool) { c.Wallpaper.MultiMonitorEnabled = v }},
		"kde.set_lock_screen":             {kind: valueBool, setBool: func(v bool) { c.KDE.SetLockScreen = v }},
		"bookmarks.enabled":               {kind: valueBool, setBool: func(v bool) { c.Bookmarks.Enabled = v }},
		"bookmarks.sync_interval":         {kind: valueInt, setInt: func(v int) { c.Bookmarks.SyncInterval = v }},
		"bookmarks.auto_cleanup":          {kind: valueBool, setBool: func(v bool) { c.Bookmarks.AutoCleanup = v }},
	}
}

// Set updates a single dotted configuration key and returns a human-readable
// error when the key or value is invalid. Values that fall below the
// configured minimums are clamped by Validate.
func (c *Config) Set(key, value string) error {
	setter, ok := c.setters()[key]
	if !ok {
		return fmt.Errorf("unknown config key %q (see 'kpixivctl config set --help')", key)
	}

	switch setter.kind {
	case valueInt:
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s expects a whole number, got %q", key, value)
		}
		setter.setInt(v)
	case valueBool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s expects a boolean value (true/false), got %q", key, value)
		}
		setter.setBool(v)
	case valueEnum:
		for _, allowed := range setter.allowed {
			if value == allowed {
				setter.setStr(value)
				return nil
			}
		}
		return fmt.Errorf("invalid value %q for %s (must be one of: %s)", value, key, strings.Join(setter.allowed, ", "))
	}
	return nil
}

// Keys returns the sorted list of keys supported by Set.
func Keys() []string {
	registry := (&Config{}).setters()
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func resolveDefaultDownloadPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return DefaultDownloadPath
	}

	return filepath.Join(homeDir, DefaultDownloadPath)
}
