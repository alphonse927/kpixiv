package fetcher

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/testutil"
	"github.com/alphonse927/kpixiv/internal/testutil/pixivtest"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.RunMain(m))
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

func TestDownloadAndSaveSkipsMissingOutputFiles(t *testing.T) {
	st := testutil.NewStorage(t)
	f := NewFetcher(testFetcherConfig(), st, &pixivtest.Client{})

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

	f := NewFetcher(cfg, testutil.NewStorage(t), &pixivtest.Client{})
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

	f := NewFetcher(cfg, testutil.NewStorage(t), &pixivtest.Client{})
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
	f := NewFetcher(cfg, testutil.NewStorage(t), &pixivtest.Client{})

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

func TestFetchQueuesIntoGlobalQueueByDefault(t *testing.T) {
	st := testutil.NewStorage(t)
	images := []pixiv.Image{{ID: "a", Width: 1920, Height: 1080}}
	f := NewFetcher(testFetcherConfig(), st, pixivtest.NewWriting(images))

	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	q := storage.NewQueue(st.StateDir())
	if err := q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if got := q.GetAll(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("global queue: got %v, want [a]", got)
	}
}

func TestFetchUsesCustomEnqueue(t *testing.T) {
	st := testutil.NewStorage(t)
	images := []pixiv.Image{{ID: "a", Width: 1920, Height: 1080}}
	f := NewFetcher(testFetcherConfig(), st, pixivtest.NewWriting(images))

	var received []string
	f.SetEnqueue(func(ids []string) error {
		received = append(received, ids...)

		return nil
	})

	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if !slices.Equal(received, []string{"a"}) {
		t.Fatalf("custom enqueue received %v, want [a]", received)
	}

	// The global queue must stay untouched when a custom destination is set.
	q := storage.NewQueue(st.StateDir())
	if err := q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !q.IsEmpty() {
		t.Fatalf("global queue should be empty, got %v", q.GetAll())
	}
}
