package app

import (
	"path/filepath"
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
)

func TestMain(m *testing.M) {
	logger.Init(false)
	m.Run()
}

func TestStartStopsSchedulerOnInitialFetchFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		DownloadPath: filepath.Join(tmp, "downloads"),
		Pixiv: config.PixivConfig{
			MinWidth:  1280,
			MinHeight: 720,
			Ranking:   config.RankingDailyMode,
		},
		Wallpaper: config.WallpaperConfig{
			SetInterval:   60,
			FetchInterval: 60,
			HistoryLimit:  10,
			CleanupDays:   7,
		},
	}

	controller, err := New(cfg, true, false)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	err = controller.Start()
	if err == nil {
		t.Fatal("Start() returned nil, want error")
	}

	if controller.sch.IsRunning() {
		t.Fatal("scheduler should stop after startup failure")
	}
}
