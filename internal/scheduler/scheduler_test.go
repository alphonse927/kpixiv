package scheduler

import (
	"context"
	"os"
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

type mockSetter struct {
	setCalled bool
	lastPath  string
}

func (m *mockSetter) Set(path string) error {
	m.setCalled = true
	m.lastPath = path
	return nil
}

type mockPixivClient struct {
	images      []pixiv.Image
	nextPage    int
	fetchErr    error
	downloadErr error
}

func (m *mockPixivClient) FetchRanking(context.Context, string, int, bool) ([]pixiv.Image, int, error) {
	if m.fetchErr != nil {
		return nil, 1, m.fetchErr
	}
	return m.images, m.nextPage, nil
}

func (m *mockPixivClient) DownloadImage(context.Context, *pixiv.Image, string) error {
	return m.downloadErr
}

func testStorage(t *testing.T) *storage.Storage {
	tmp := t.TempDir()
	downloadDir := filepath.Join(tmp, "downloads")
	s, err := storage.New(tmp, downloadDir)
	if err != nil {
		t.Fatalf("NewForTest() returned error: %v", err)
	}
	return s
}

func testConfig() *config.Config {
	return &config.Config{
		DownloadPath: "/tmp/downloads",
		Pixiv: config.PixivConfig{
			MinWidth:  1280,
			MinHeight: 720,
			Ranking:   config.RankingDailyMode,
			R18:       false,
		},
		Wallpaper: config.WallpaperConfig{
			SetInterval:   60,
			FetchInterval: 60,
		},
	}
}

func TestNew(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)
	if sch == nil {
		t.Fatal("New() returned nil")
	}
}

func TestRunAlreadyRunning(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)
	ctx := context.Background()

	err := sch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() first time: got error %v", err)
	}

	err = sch.Run(ctx)
	if err == nil {
		t.Error("Run() second time: got nil, want error")
	}

	sch.Stop("test")
}

func TestStop(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)
	ctx := context.Background()

	_ = sch.Run(ctx)
	sch.Stop("test")

	if sch.IsRunning() {
		t.Error("IsRunning() after Stop(): got true, want false")
	}
}

func TestStopMultipleTimes(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)
	ctx := context.Background()

	_ = sch.Run(ctx)
	sch.Stop("test")
	sch.Stop("test")
}

func TestSetNextNoWallpapers(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}
	q := storage.NewQueue(s.StateDir())

	sch := New(cfg, s, m, setter)
	if err := sch.SetNextWallpaper(q, "test"); err == nil {
		t.Error("SetNext() with no wallpapers: got nil, want err")
	}

	if setter.setCalled {
		t.Error("SetNext() with no wallpapers: setCalled = true, want false")
	}
}

func TestSetNextWithWallpapers(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}
	q := storage.NewQueue(s.StateDir())

	firstPath := filepath.Join(s.RankingDir(), "img1.jpg")
	secondPath := filepath.Join(s.RankingDir(), "img2.jpg")
	if err := os.WriteFile(firstPath, []byte("img1"), 0600); err != nil {
		t.Fatalf("WriteFile(img1) returned error: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("img2"), 0600); err != nil {
		t.Fatalf("WriteFile(img2) returned error: %v", err)
	}

	if err := s.SaveMetadata(map[string]*storage.ImageMeta{
		"img1": {ID: "img1", Path: firstPath},
		"img2": {ID: "img2", Path: secondPath},
	}); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	if err := q.AppendRandom([]string{"img1", "img2"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if err := s.ExcludeWallpaper("img1"); err != nil {
		t.Fatalf("ExcludeWallpaper() returned error: %v", err)
	}

	sch := New(cfg, s, m, setter)
	if err := sch.SetNextWallpaper(q, "test"); err != nil {
		t.Fatalf("SetNextWallpaper() returned error: %v", err)
	}

	if !setter.setCalled {
		t.Fatal("SetNextWallpaper() should set a wallpaper")
	}

	if setter.lastPath != secondPath {
		t.Fatalf("SetNextWallpaper() path: got %q, want %q", setter.lastPath, secondPath)
	}
}

func TestSetNextAddsToHistory(t *testing.T) {
	t.Skip("SetNext requires queue setup")
}

func TestSetNextIsRandom(t *testing.T) {
	t.Skip("SetNext requires queue setup")
}

func TestApplyCurrentOrNextSkipsBlacklistedCurrent(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	firstPath := filepath.Join(s.RankingDir(), "img1.jpg")
	secondPath := filepath.Join(s.RankingDir(), "img2.jpg")
	if err := os.WriteFile(firstPath, []byte("img1"), 0600); err != nil {
		t.Fatalf("WriteFile(img1) returned error: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("img2"), 0600); err != nil {
		t.Fatalf("WriteFile(img2) returned error: %v", err)
	}

	if err := s.SaveMetadata(map[string]*storage.ImageMeta{
		"img1": {ID: "img1", Path: firstPath},
		"img2": {ID: "img2", Path: secondPath},
	}); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	if err := s.SaveHistory(&storage.History{Current: "img1", Images: []string{"img1", "img2"}}); err != nil {
		t.Fatalf("SaveHistory() returned error: %v", err)
	}

	if err := s.ExcludeWallpaper("img1"); err != nil {
		t.Fatalf("ExcludeWallpaper() returned error: %v", err)
	}

	sch := New(cfg, s, m, setter)
	if err := sch.ApplyCurrentOrNext(); err != nil {
		t.Fatalf("ApplyCurrentOrNext() returned error: %v", err)
	}

	if setter.lastPath != secondPath {
		t.Fatalf("ApplyCurrentOrNext() path: got %q, want %q", setter.lastPath, secondPath)
	}
}

func TestIsRunning(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)

	if sch.IsRunning() {
		t.Error("IsRunning() before Run(): got true, want false")
	}

	ctx := context.Background()
	_ = sch.Run(ctx)

	if !sch.IsRunning() {
		t.Error("IsRunning() during Run(): got false, want true")
	}

	sch.Stop("test")

	if sch.IsRunning() {
		t.Error("IsRunning() after Stop(): got true, want false")
	}
}
