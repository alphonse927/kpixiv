package config

import "testing"

func TestSet(t *testing.T) {
	cfg := Default()

	cases := []struct {
		key   string
		value string
		check func(*Config) bool
	}{
		{"pixiv.r18", "true", func(c *Config) bool { return c.Pixiv.R18 }},
		{"pixiv.r18", "false", func(c *Config) bool { return !c.Pixiv.R18 }},
		{"pixiv.min_width", "2560", func(c *Config) bool { return c.Pixiv.MinWidth == 2560 }},
		{"pixiv.min_height", "1440", func(c *Config) bool { return c.Pixiv.MinHeight == 1440 }},
		{"wallpaper.set_interval", "15", func(c *Config) bool { return c.Wallpaper.SetInterval == 15 }},
		{"wallpaper.rotation_enabled", "true", func(c *Config) bool { return c.Wallpaper.RotationEnabled }},
		{"wallpaper.multi_monitor_enabled", "true", func(c *Config) bool { return c.Wallpaper.MultiMonitorEnabled }},
		{"kde.set_lock_screen", "true", func(c *Config) bool { return c.KDE.SetLockScreen }},
		{"bookmarks.enabled", "true", func(c *Config) bool { return c.Bookmarks.Enabled }},
		{"bookmarks.sync_interval", "120", func(c *Config) bool { return c.Bookmarks.SyncInterval == 120 }},
		{"log_level", "debug", func(c *Config) bool { return c.LogLevel == "debug" }},
	}

	for _, tc := range cases {
		if err := cfg.Set(tc.key, tc.value); err != nil {
			t.Errorf("Set(%q, %q) returned error: %v", tc.key, tc.value, err)
			continue
		}
		if !tc.check(cfg) {
			t.Errorf("Set(%q, %q) did not apply the value", tc.key, tc.value)
		}
	}
}

func TestSetErrors(t *testing.T) {
	cfg := Default()

	if err := cfg.Set("unknown.key", "x"); err == nil {
		t.Error("Set() accepted an unknown key")
	}

	if err := cfg.Set("pixiv.r18", "notabool"); err == nil {
		t.Error("Set() accepted a non-boolean for a bool key")
	}

	if err := cfg.Set("pixiv.min_width", "wide"); err == nil {
		t.Error("Set() accepted a non-integer for an int key")
	}

	if err := cfg.Set("log_level", "verbose"); err == nil {
		t.Error("Set() accepted an invalid log level")
	}
}
