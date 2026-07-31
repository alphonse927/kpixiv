package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	if cfg.Pixiv.Ranking != RankingDailyMode {
		t.Errorf("Default() Ranking: got %s, want %s", cfg.Pixiv.Ranking, RankingDailyMode)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		expected := filepath.Join(homeDir, DefaultDownloadPath)
		if cfg.DownloadPath != expected {
			t.Errorf("Default() DownloadPath: got %s, want %s", cfg.DownloadPath, expected)
		}
	}

	if cfg.KDE.SetLockScreen {
		t.Errorf("Default() KDE.SetLockScreen: got %v, want false", cfg.KDE.SetLockScreen)
	}
	if cfg.Wallpaper.CleanupDays != DefaultCleanupDays {
		t.Errorf("Default() Wallpaper.CleanupDays: got %d, want %d", cfg.Wallpaper.CleanupDays, DefaultCleanupDays)
	}
	if cfg.Wallpaper.HistoryLimit != DefaultHistoryLimit {
		t.Errorf("Default() Wallpaper.HistoryLimit: got %d, want %d", cfg.Wallpaper.HistoryLimit, DefaultHistoryLimit)
	}
}

func TestLoadCreatesDefault(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	if cfg.Pixiv.Ranking != RankingDailyMode {
		t.Errorf("Load() default Ranking: got %s, want %s", cfg.Pixiv.Ranking, RankingDailyMode)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file was not created at %s", cfgPath)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	original := Default()
	original.Pixiv.Ranking = RankingWeeklyMode
	original.Pixiv.R18 = true
	original.Pixiv.MinWidth = 2560
	original.Pixiv.MinHeight = 1440

	if err := Save(cfgPath, original); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if loaded.Pixiv.Ranking != RankingWeeklyMode {
		t.Errorf("Load() Ranking: got %s, want %s", loaded.Pixiv.Ranking, RankingWeeklyMode)
	}
	if loaded.Pixiv.R18 != true {
		t.Errorf("Load() R18: got %v, want true", loaded.Pixiv.R18)
	}
	if loaded.Pixiv.MinWidth != 2560 {
		t.Errorf("Load() MinWidth: got %d, want 2560", loaded.Pixiv.MinWidth)
	}
}

func TestValidate(t *testing.T) {
	cfg := Default()

	if issues := cfg.Validate(); len(issues) != 0 {
		t.Errorf("Validate() reported unexpected issues for valid config: %v", issues)
	}

	originalMinWidth := cfg.Pixiv.MinWidth
	cfg.Pixiv.MinWidth = 0
	issues := cfg.Validate()
	if cfg.Pixiv.MinWidth != DefaultMinWidth {
		t.Errorf("Validate() clamped MinWidth: got %d, want %d", cfg.Pixiv.MinWidth, DefaultMinWidth)
	}
	if len(issues) == 0 {
		t.Error("Validate() did not report a MinWidth adjustment")
	}
	cfg.Pixiv.MinWidth = originalMinWidth

	originalMinHeight := cfg.Pixiv.MinHeight
	cfg.Pixiv.MinHeight = 0
	if issues := cfg.Validate(); len(issues) == 0 {
		t.Error("Validate() did not report a MinHeight adjustment")
	}
	if cfg.Pixiv.MinHeight != DefaultMinHeight {
		t.Errorf("Validate() clamped MinHeight: got %d, want %d", cfg.Pixiv.MinHeight, DefaultMinHeight)
	}
	cfg.Pixiv.MinHeight = originalMinHeight

	cfg.Wallpaper.CleanupDays = 0
	if issues := cfg.Validate(); len(issues) == 0 {
		t.Error("Validate() did not report a CleanupDays adjustment")
	}
	if cfg.Wallpaper.CleanupDays != DefaultCleanupDays {
		t.Errorf("Validate() clamped CleanupDays: got %d, want %d", cfg.Wallpaper.CleanupDays, DefaultCleanupDays)
	}

	cfg.Wallpaper.HistoryLimit = 0
	if issues := cfg.Validate(); len(issues) == 0 {
		t.Error("Validate() did not report a HistoryLimit adjustment")
	}
	if cfg.Wallpaper.HistoryLimit != DefaultHistoryLimit {
		t.Errorf("Validate() clamped HistoryLimit: got %d, want %d", cfg.Wallpaper.HistoryLimit, DefaultHistoryLimit)
	}
}

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
