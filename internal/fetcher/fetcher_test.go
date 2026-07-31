package fetcher

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
)

func TestMain(m *testing.M) {
	logger.Init(false)
	m.Run()
}

type mockImageClient struct {
	fetchImages  []pixiv.Image
	nextPage     int
	downloadFunc func(context.Context, *pixiv.Image, string) error
}

func (m *mockImageClient) FetchRanking(context.Context, string, int, bool) ([]pixiv.Image, int, error) {
	return m.fetchImages, m.nextPage, nil
}

func (m *mockImageClient) DownloadImage(ctx context.Context, img *pixiv.Image, destPath string) error {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, img, destPath)
	}
	return nil
}

func testFetcherConfig() *config.Config {
	return &config.Config{
		Pixiv: config.PixivConfig{
			MinWidth:  1280,
			MinHeight: 720,
			Ranking:   config.RankingDailyMode,
		},
		Wallpaper: config.WallpaperConfig{
			Orientation: config.WallpaperAnyOrientation,
		},
	}
}

func testFetcherStorage(t *testing.T) *storage.Storage {
	t.Helper()

	tmp := t.TempDir()
	st, err := storage.New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("storage.New() returned error: %v", err)
	}

	return st
}

func TestDownloadAndSaveSkipsMissingOutputFiles(t *testing.T) {
	st := testFetcherStorage(t)
	f := NewFetcher(testFetcherConfig(), st, &mockImageClient{})

	img := pixiv.Image{ID: "12345", Width: 1920, Height: 1080}
	metadata := map[string]*storage.ImageMeta{}

	downloaded := f.downloadAndSave(context.Background(), []pixiv.Image{img}, metadata)
	if len(downloaded) != 0 {
		t.Fatalf("downloadAndSave() downloaded count: got %d, want 0", len(downloaded))
	}

	if _, ok := metadata[img.ID]; ok {
		t.Fatalf("downloadAndSave() should not persist metadata for missing files")
	}

	loaded, err := st.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() returned error: %v", err)
	}

	if _, ok := loaded[img.ID]; ok {
		t.Fatalf("saved metadata should not include %s", img.ID)
	}
}

func TestFilterImagesAllowsPortraitForPortraitMonitor(t *testing.T) {
	cfg := testFetcherConfig()
	cfg.Wallpaper.MultiMonitorEnabled = true
	cfg.Wallpaper.Monitors = map[string]config.MonitorConfig{
		"0": {Orientation: config.WallpaperPortraitOrientation},
	}
	f := NewFetcher(cfg, testFetcherStorage(t), &mockImageClient{})

	images := f.filterImages([]pixiv.Image{
		{ID: "portrait", Width: 1280, Height: 2000},
		{ID: "landscape", Width: 2000, Height: 1280},
	}, map[string]struct{}{})
	if len(images) != 2 {
		t.Fatalf("filterImages() returned %d images, want both orientations", len(images))
	}
}

func TestFilterImagesAllowsPortraitForAnyMonitor(t *testing.T) {
	cfg := testFetcherConfig()
	cfg.Wallpaper.MultiMonitorEnabled = true
	cfg.Wallpaper.Monitors = map[string]config.MonitorConfig{
		"0": {Orientation: config.WallpaperAnyOrientation},
	}
	f := NewFetcher(cfg, testFetcherStorage(t), &mockImageClient{})

	images := f.filterImages([]pixiv.Image{
		{ID: "portrait", Width: 1280, Height: 2000},
		{ID: "landscape", Width: 2000, Height: 1280},
	}, map[string]struct{}{})
	if len(images) != 2 {
		t.Fatalf("filterImages() returned %d images, want both orientations", len(images))
	}
}

func TestFilterImagesSingleMonitorUsesWallpaperOrientation(t *testing.T) {
	cfg := testFetcherConfig()
	cfg.Wallpaper.Orientation = config.WallpaperPortraitOrientation
	f := NewFetcher(cfg, testFetcherStorage(t), &mockImageClient{})

	images := f.filterImages([]pixiv.Image{
		{ID: "portrait", Width: 1280, Height: 2000},
		{ID: "landscape", Width: 2000, Height: 1280},
	}, map[string]struct{}{})
	if len(images) != 1 {
		t.Fatalf("filterImages() returned %d images, want 1 portrait image", len(images))
	}
	if images[0].ID != "portrait" {
		t.Fatalf("filterImages() returned %q, want portrait", images[0].ID)
	}
}
