package app

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/notify"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
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

func TestIsWallpaperBookmarked(t *testing.T) {
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

	if controller.IsWallpaperBookmarked("") {
		t.Fatal("IsWallpaperBookmarked(\"\") = true, want false")
	}

	if controller.CurrentWallpaperID() != "" {
		t.Fatalf("CurrentWallpaperID() = %q, want \"\" (no current wallpaper)", controller.CurrentWallpaperID())
	}

	if controller.IsWallpaperBookmarked("12345") {
		t.Fatal("unknown artwork should not be bookmarked")
	}

	if err := controller.st.AddBookmark("12345"); err != nil {
		t.Fatalf("AddBookmark() returned error: %v", err)
	}

	if !controller.IsWallpaperBookmarked("12345") {
		t.Fatal("bookmarked artwork should report bookmarked")
	}
}

// fakeSetter records wallpaper changes, both global and per screen.
type fakeSetter struct {
	screens   []wallpaper.Screen
	globalSet int
	applied   []string // screen indexes, in order
}

func (f *fakeSetter) Set(string) error {
	f.globalSet++

	return nil
}

func (f *fakeSetter) Screens() ([]wallpaper.Screen, error) { return f.screens, nil }

func (f *fakeSetter) SetForScreen(index, _ string) error {
	f.applied = append(f.applied, index)

	return nil
}

// exclusionFixture builds a controller over a throwaway storage holding the
// artworks "bad", "good", "x", "y" and "z", all with a file, metadata and thumbnail.
func exclusionFixture(t *testing.T, multi bool, setter wallpaper.Setter) *Controller {
	t.Helper()

	notify.SetEnabled(false)
	t.Cleanup(func() { notify.SetEnabled(true) })

	tmp := t.TempDir()
	st, err := storage.New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("storage.New() returned error: %v", err)
	}

	meta := map[string]*storage.ImageMeta{}
	for _, id := range []string{"bad", "good", "x", "y", "z"} {
		path := filepath.Join(st.RankingDir(), id+".jpg")
		if err = os.WriteFile(path, []byte(id), 0600); err != nil {
			t.Fatalf("WriteFile() returned error: %v", err)
		}
		if err = os.WriteFile(st.ThumbnailPath(id), []byte("thumb"), 0600); err != nil {
			t.Fatalf("WriteFile() returned error: %v", err)
		}
		meta[id] = &storage.ImageMeta{ID: id, Path: path}
	}
	if err = st.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	cfg := &config.Config{Wallpaper: config.WallpaperConfig{
		MultiMonitorEnabled: multi,
		HistoryLimit:        10,
	}}

	return &Controller{
		cfg:                 cfg,
		st:                  st,
		sch:                 scheduler.New(cfg, st, nil, setter),
		setter:              setter,
		multiMonitorEnabled: multi,
	}
}

func seedQueues(t *testing.T, st *storage.Storage) {
	t.Helper()

	queues := map[string][]string{
		"":       {"bad", "x"},
		"DP-1":   {"bad", "y"},
		"HDMI-1": {"bad", "z"},
	}
	for monitorID, items := range queues {
		q := storage.NewQueue(st.StateDir())
		if monitorID != "" {
			q = storage.NewMonitorQueue(st.StateDir(), monitorID)
		}
		if err := q.AppendRandom(items); err != nil {
			t.Fatalf("AppendRandom() returned error: %v", err)
		}
	}
}

func TestExcludeWallpaperInMultiMonitorModeOnlyChangesTheScreenShowingIt(t *testing.T) {
	setter := &fakeSetter{screens: []wallpaper.Screen{
		{ID: "DP-1", Index: "0"},
		{ID: "HDMI-1", Index: "1"},
	}}
	c := exclusionFixture(t, true, setter)
	st := c.st
	seedQueues(t, st)
	if err := st.AddToMonitorHistory("DP-1", "bad", 10); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}
	if err := st.AddToMonitorHistory("HDMI-1", "good", 10); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}

	if err := c.ExcludeWallpaper("bad"); err != nil {
		t.Fatalf("ExcludeWallpaper() returned error: %v", err)
	}

	if !slices.Equal(setter.applied, []string{"0"}) {
		t.Errorf("screens that changed wallpaper: got %v, want only [0] (DP-1)", setter.applied)
	}
	if setter.globalSet != 0 {
		t.Errorf("the global wallpaper must not be set in multi-monitor mode, got %d calls", setter.globalSet)
	}

	history, err := st.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if history.Monitors["HDMI-1"] != "good" {
		t.Errorf("HDMI-1 must keep its wallpaper, got %q", history.Monitors["HDMI-1"])
	}
	if got := history.Monitors["DP-1"]; got == "" || got == "bad" {
		t.Errorf("DP-1 should show a new wallpaper, got %q", got)
	}
	if slices.Contains(history.Images, "bad") {
		t.Errorf("the excluded artwork must not come back to the history list: %v", history.Images)
	}

	assertExcluded(t, st, "bad")
}

func TestExcludeWallpaperNotShownOnAnyScreenChangesNothing(t *testing.T) {
	setter := &fakeSetter{screens: []wallpaper.Screen{
		{ID: "DP-1", Index: "0"},
		{ID: "HDMI-1", Index: "1"},
	}}
	c := exclusionFixture(t, true, setter)
	seedQueues(t, c.st)
	if err := c.st.AddToMonitorHistory("DP-1", "good", 10); err != nil {
		t.Fatalf("AddToMonitorHistory() returned error: %v", err)
	}

	if err := c.ExcludeWallpaper("bad"); err != nil {
		t.Fatalf("ExcludeWallpaper() returned error: %v", err)
	}

	if len(setter.applied) != 0 || setter.globalSet != 0 {
		t.Errorf("no wallpaper should change: per-screen=%v global=%d", setter.applied, setter.globalSet)
	}
	assertExcluded(t, c.st, "bad")
}

func TestExcludeWallpaperInSingleMonitorModeChangesTheGlobalWallpaper(t *testing.T) {
	setter := &fakeSetter{}
	c := exclusionFixture(t, false, setter)
	seedQueues(t, c.st)

	if err := c.ExcludeWallpaper("bad"); err != nil {
		t.Fatalf("ExcludeWallpaper() returned error: %v", err)
	}

	if setter.globalSet != 1 {
		t.Errorf("global wallpaper changes: got %d, want 1", setter.globalSet)
	}
	assertExcluded(t, c.st, "bad")
}

// assertExcluded checks that an excluded artwork is gone from everywhere
// except the blacklist, which must now list it.
func assertExcluded(t *testing.T, st *storage.Storage, id string) {
	t.Helper()

	blacklist, err := st.LoadBlacklist()
	if err != nil {
		t.Fatalf("LoadBlacklist() returned error: %v", err)
	}
	if !blacklist.Contains(id) {
		t.Errorf("%s should be blacklisted", id)
	}

	if _, ok := st.GetImagePath(id); ok {
		t.Errorf("the file of %s should be deleted", id)
	}
	if _, statErr := os.Stat(st.ThumbnailPath(id)); !os.IsNotExist(statErr) {
		t.Errorf("the thumbnail of %s should be deleted, stat error: %v", id, statErr)
	}

	metadata, err := st.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() returned error: %v", err)
	}
	if _, ok := metadata[id]; ok {
		t.Errorf("metadata of %s should be removed", id)
	}

	for _, monitorID := range []string{"", "DP-1", "HDMI-1"} {
		q := storage.NewQueue(st.StateDir())
		if monitorID != "" {
			q = storage.NewMonitorQueue(st.StateDir(), monitorID)
		}
		if err = q.Load(); err != nil {
			t.Fatalf("Load(%q) returned error: %v", monitorID, err)
		}
		items := q.GetAll()
		sort.Strings(items)
		if slices.Contains(items, id) {
			t.Errorf("queue %q still lists %s: %v", monitorID, id, items)
		}
	}
}
