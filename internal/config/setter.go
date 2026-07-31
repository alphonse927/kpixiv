package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

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

// setters return the registry of supported configuration keys.
func (c *Config) setters() map[string]keySetter {
	return map[string]keySetter{
		"log_level":             {kind: valueEnum, setStr: func(v string) { c.LogLevel = v }, allowed: []string{"debug", "info", "warn", "error"}},
		"notifications.enabled": {kind: valueBool, setBool: func(v bool) { c.Notifications.Enabled = v }},
		"pixiv.r18":             {kind: valueBool, setBool: func(v bool) { c.Pixiv.R18 = v }},
		"pixiv.ranking":         {kind: valueInt, setInt: func(v int) { c.Pixiv.Ranking = RankingMode(v) }},
		"pixiv.min_width":       {kind: valueInt, setInt: func(v int) { c.Pixiv.MinWidth = v }},
		"pixiv.min_height":      {kind: valueInt, setInt: func(v int) { c.Pixiv.MinHeight = v }},
		"wallpaper.orientation": {kind: valueEnum, setStr: func(v string) { c.Wallpaper.Orientation = WallpaperOrientation(v) },
			allowed: []string{WallpaperAnyOrientation.String(), WallpaperLandscapeOrientation.String(), WallpaperPortraitOrientation.String()}},
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
		if slices.Contains(setter.allowed, value) {
			setter.setStr(value)
			return nil
		}

		return fmt.Errorf("invalid value %q for %s (must be one of: %s)", value, key, strings.Join(setter.allowed, ", "))
	}
	return nil
}
