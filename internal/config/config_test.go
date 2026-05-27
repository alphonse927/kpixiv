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
	cfg := &Config{
		Pixiv: PixivConfig{
			MinWidth:  1280,
			MinHeight: 720,
			Ranking:   RankingDailyMode,
			R18:       false,
		},
		Wallpaper: WallpaperConfig{
			SetInterval:   30,
			FetchInterval: 60,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with valid config: got error %v, want nil", err)
	}

	originalMinWidth := cfg.Pixiv.MinWidth
	cfg.Pixiv.MinWidth = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with MinWidth=0: should clamp, got error %v", err)
	}
	if cfg.Pixiv.MinWidth != DefaultMinWidth {
		t.Errorf("Validate() clamped MinWidth: got %d, want %d", cfg.Pixiv.MinWidth, DefaultMinWidth)
	}
	cfg.Pixiv.MinWidth = originalMinWidth

	originalMinHeight := cfg.Pixiv.MinHeight
	cfg.Pixiv.MinHeight = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with MinHeight=0: should clamp, got error %v", err)
	}
	if cfg.Pixiv.MinHeight != DefaultMinHeight {
		t.Errorf("Validate() clamped MinHeight: got %d, want %d", cfg.Pixiv.MinHeight, DefaultMinHeight)
	}
	cfg.Pixiv.MinHeight = originalMinHeight

	cfg.Wallpaper.CleanupDays = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with CleanupDays=0: should clamp, got error %v", err)
	}
	if cfg.Wallpaper.CleanupDays != DefaultCleanupDays {
		t.Errorf("Validate() clamped CleanupDays: got %d, want %d", cfg.Wallpaper.CleanupDays, DefaultCleanupDays)
	}

	cfg.Wallpaper.HistoryLimit = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with HistoryLimit=0: should clamp, got error %v", err)
	}
	if cfg.Wallpaper.HistoryLimit != DefaultHistoryLimit {
		t.Errorf("Validate() clamped HistoryLimit: got %d, want %d", cfg.Wallpaper.HistoryLimit, DefaultHistoryLimit)
	}
}
