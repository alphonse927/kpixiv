package scheduler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
)

func TestMain(m *testing.M) {
	// logger.Init writes to a centralized log file under $HOME/.local/state
	// (see internal/platform.LogFilePath). Point HOME at a throwaway temp
	// dir for the whole test binary run so tests never touch the real
	// user's state directory.
	tmpHome, err := os.MkdirTemp("", "kpixiv-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", tmpHome) //nolint:errcheck
	logger.Init(false)
	code := m.Run()
	os.RemoveAll(tmpHome) //nolint:errcheck
	os.Exit(code)
}

func TestMatchesOrientation(t *testing.T) {
	landscape := &storage.ImageMeta{Width: 1920, Height: 1080}
	portrait := &storage.ImageMeta{Width: 1080, Height: 1920}

	if !matchesOrientation(landscape, config.WallpaperLandscapeOrientation) {
		t.Error("landscape image did not match landscape orientation")
	}
	if matchesOrientation(landscape, config.WallpaperPortraitOrientation) {
		t.Error("landscape image matched portrait orientation")
	}
	if !matchesOrientation(portrait, config.WallpaperPortraitOrientation) {
		t.Error("portrait image did not match portrait orientation")
	}
	if !matchesOrientation(landscape, config.WallpaperAnyOrientation) {
		t.Error("landscape image did not match any orientation")
	}
}

type mockSetter struct {
	setCalled bool
	lastPath  string
	onSet     func(string)
}

func (m *mockSetter) Set(path string) error {
	m.setCalled = true
	m.lastPath = path
	if m.onSet != nil {
		m.onSet(path)
	}
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
			SetInterval:     60,
			FetchInterval:   60,
			RotationEnabled: true,
			FetchEnabled:    true,
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

func TestSetNextReturnsImageNotFoundWhenNoValidWallpaperApplied(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}
	q := storage.NewQueue(s.StateDir())

	if err := q.AppendRandom([]string{"missing-1", "missing-2", "missing-3", "missing-4", "missing-5"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	sch := New(cfg, s, m, setter)
	err := sch.SetNextWallpaper(q, "test")
	if !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("SetNextWallpaper() error: got %v, want ErrImageNotFound", err)
	}

	if setter.setCalled {
		t.Fatal("SetNextWallpaper() should not set a wallpaper")
	}
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

func TestFetchNowTriggersFetchWhileRunning(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	setter := &mockSetter{}
	var calls atomic.Int32
	m := &mockPixivClient{
		images:   []pixiv.Image{},
		nextPage: 2,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &mockPixivClientWithHook{mockPixivClient: m, onFetch: func() {
		calls.Add(1)
	}}

	sch := New(cfg, s, client, setter)
	if err := sch.Run(ctx); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	defer sch.Stop("test")

	// The scheduler now also fetches once immediately on startup (see
	// run()). Wait for that initial call and reset the counter so this
	// test specifically verifies FetchNow(), not just that some fetch
	// happened since Run() was called.
	deadline := time.After(2 * time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("expected an initial fetch on startup")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	calls.Store(0)

	sch.FetchNow(ctx, "test")

	deadline = time.After(2 * time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("FetchNow() did not trigger a fetch while running")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestResetRotationTimerDelaysNextScheduledChange(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	setCalls := make(chan time.Time, 4)
	setter := &mockSetter{onSet: func(string) {
		setCalls <- time.Now()
	}}
	m := &mockPixivClient{}
	q := storage.NewQueue(s.StateDir())

	for _, id := range []string{"img1", "img2", "img3"} {
		path := filepath.Join(s.RankingDir(), id+".jpg")
		if err := os.WriteFile(path, []byte(id), 0600); err != nil {
			t.Fatalf("WriteFile(%s) returned error: %v", id, err)
		}
	}

	if err := s.SaveMetadata(map[string]*storage.ImageMeta{
		"img1": {ID: "img1", Path: filepath.Join(s.RankingDir(), "img1.jpg")},
		"img2": {ID: "img2", Path: filepath.Join(s.RankingDir(), "img2.jpg")},
		"img3": {ID: "img3", Path: filepath.Join(s.RankingDir(), "img3.jpg")},
	}); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	if err := q.AppendRandom([]string{"img1", "img2", "img3"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	sch := New(cfg, s, m, setter)
	sch.setInterval = 200 * time.Millisecond
	sch.fetchInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sch.Run(ctx); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	defer sch.Stop("test")

	time.Sleep(120 * time.Millisecond)

	if err := sch.SetNextWallpaper(q, "test"); err != nil {
		t.Fatalf("SetNextWallpaper() returned error: %v", err)
	}
	sch.ResetRotationTimer()

	manualAt := <-setCalls
	autoAt := <-setCalls

	if delta := autoAt.Sub(manualAt); delta < 150*time.Millisecond {
		t.Fatalf("scheduled wallpaper changed too soon after manual change: got %v, want at least %v", delta, 150*time.Millisecond)
	}
}

type mockPixivClientWithHook struct {
	*mockPixivClient
	onFetch func()
}

func (m *mockPixivClientWithHook) FetchRanking(ctx context.Context, mode string, page int, r18 bool) ([]pixiv.Image, int, error) {
	if m.onFetch != nil {
		m.onFetch()
	}

	return m.mockPixivClient.FetchRanking(ctx, mode, page, r18)
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

// TestBookmarkSyncStartsAfterBeingEnabledPostStartup is a regression test for
// a bug where the bookmark-sync ticker was only created if Bookmarks.Enabled
// was already true at the moment the scheduler started. If a user enabled
// bookmark sync later (e.g. after logging in via Settings), the running
// scheduler's ticker channel stayed nil forever and bookmark sync never ran
// again until the process was restarted -- surfaced in the GUI as "Next
// sync: Any moment now" stuck indefinitely. It's fixed by always creating
// the ticker and gating the sync on the live Enabled flag inside the tick
// case, the same pattern already used for setTicker/fetchTicker.
func TestBookmarkSyncStartsAfterBeingEnabledPostStartup(t *testing.T) {
	cfg := testConfig()
	cfg.Bookmarks.Enabled = false // disabled when the scheduler starts
	s := testStorage(t)
	m := &mockPixivClient{}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)
	sch.bookmarkSyncInterval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sch.Run(ctx); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	defer sch.Stop("test")

	logger.SetLevel("debug")
	defer logger.SetLevel("info")

	// Observe the debug logs emitted by syncBookmarks via the centralized
	// log file (see internal/logger and internal/platform.LogFilePath) to
	// confirm the tick loop actually attempts a sync once enabled. TestMain
	// points HOME at a throwaway temp dir, so this file lives there, not in
	// the real user's state directory.
	logPath := logger.FilePath()
	if logPath == "" {
		t.Fatal("logger.FilePath() is empty; centralized log file was not initialized")
	}

	var startOffset int64
	if info, err := os.Stat(logPath); err == nil {
		startOffset = info.Size()
	}

	// Enable bookmark sync the way Settings would after a save: it mutates
	// the shared config pointer that the running scheduler already holds
	// (internal/app/app.go documents this pattern). Note this deliberately
	// does NOT call ApplyConfig: the interval is left as the 20ms override
	// above rather than being recomputed from cfg.Bookmarks.SyncInterval
	// (whole minutes), which would be far too coarse for this test and,
	// at its zero-value default, would panic on ticker.Reset(0). The point
	// of this test is specifically that the *already-running* ticker picks
	// up the live Enabled flag -- it was never about interval changes.
	cfg.Bookmarks.Enabled = true

	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("bookmark sync was never attempted after being enabled post-startup")
		}

		data, err := os.ReadFile(logPath)
		if err == nil && int64(len(data)) > startOffset {
			if strings.Contains(string(data[startOffset:]), "Skipping bookmark sync: not logged in") {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestFetchAttemptTrackedEvenOnFailure is a regression test for a bug where
// "Next fetch" got stuck at "Any moment now" forever once fetches started
// failing (expired login, network error, etc.). storage.Activity's
// LastFetchAt only records *successful* fetches, but the GUI's countdown
// was computed solely from it -- so a string of failures left the countdown
// permanently based on a stale success timestamp, with no way to tell
// whether kPixiv was still trying. LastFetchAttempt must advance on every
// attempt, success or failure, and LastFetchError must reflect the failure.
func TestFetchAttemptTrackedEvenOnFailure(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	wantErr := errors.New("network unreachable")
	m := &mockPixivClient{fetchErr: wantErr}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)

	if !sch.LastFetchAttempt().IsZero() {
		t.Fatal("LastFetchAttempt() should be zero before any fetch has run")
	}
	if sch.LastFetchError() != nil {
		t.Fatal("LastFetchError() should be nil before any fetch has run")
	}

	before := time.Now()
	err := sch.fetchImages(context.Background(), "test")
	if err == nil {
		t.Fatal("expected fetchImages to return an error")
	}

	if sch.FetchInProgress() {
		t.Error("FetchInProgress() should be false once fetchImages has returned")
	}
	if got := sch.LastFetchAttempt(); got.Before(before) {
		t.Errorf("LastFetchAttempt() = %v, want a time at or after %v", got, before)
	}
	if got := sch.LastFetchError(); got == nil || !strings.Contains(got.Error(), "network unreachable") {
		t.Errorf("LastFetchError() = %v, want an error containing %q", got, "network unreachable")
	}
}

// TestFetchInProgressDuringFetch is a regression test for the GUI's
// "Fetching..." indicator: FetchInProgress must be true while a fetch is
// actively running, not just inferable from timestamps after the fact.
func TestFetchInProgressDuringFetch(t *testing.T) {
	cfg := testConfig()
	s := testStorage(t)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	m := &mockPixivClientWithHook{
		mockPixivClient: &mockPixivClient{},
		onFetch: func() {
			started <- struct{}{}
			<-release
		},
	}
	setter := &mockSetter{}

	sch := New(cfg, s, m, setter)

	done := make(chan error, 1)
	go func() {
		done <- sch.fetchImages(context.Background(), "test")
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch never started")
	}

	if !sch.FetchInProgress() {
		t.Error("FetchInProgress() should be true while a fetch is running")
	}

	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch never finished")
	}

	if sch.FetchInProgress() {
		t.Error("FetchInProgress() should be false once the fetch has finished")
	}
}

// TestSchedulerRunsInitialFetchAndBookmarkSyncOnStartup verifies that both
// fetching and bookmark sync are attempted immediately when the scheduler
// starts, rather than only after a full fetchInterval/bookmarkSyncInterval
// has elapsed. Without this, starting kPixiv (or logging into Pixiv) meant
// waiting up to a full interval -- potentially hours -- before anything
// happened.
func TestSchedulerRunsInitialFetchAndBookmarkSyncOnStartup(t *testing.T) {
	cfg := testConfig()
	cfg.Bookmarks.Enabled = true
	s := testStorage(t)
	setter := &mockSetter{}

	fetchCalled := make(chan struct{}, 1)
	m := &mockPixivClientWithHook{
		mockPixivClient: &mockPixivClient{},
		onFetch: func() {
			select {
			case fetchCalled <- struct{}{}:
			default:
			}
		},
	}

	sch := New(cfg, s, m, setter)
	// Intervals are set far longer than the test's timeout, so any activity
	// observed can only be attributed to the immediate startup run, not a
	// regular tick.
	sch.setInterval = time.Hour
	sch.fetchInterval = time.Hour
	sch.bookmarkSyncInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.SetLevel("debug")
	defer logger.SetLevel("info")

	logPath := logger.FilePath()
	if logPath == "" {
		t.Fatal("logger.FilePath() is empty; centralized log file was not initialized")
	}
	var startOffset int64
	if info, err := os.Stat(logPath); err == nil {
		startOffset = info.Size()
	}

	if err := sch.Run(ctx); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	defer sch.Stop("test")

	select {
	case <-fetchCalled:
	case <-time.After(2 * time.Second):
		t.Error("expected an immediate fetch on startup, none observed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("expected an immediate bookmark sync attempt on startup, never observed in logs")
		}
		data, err := os.ReadFile(logPath)
		if err == nil && int64(len(data)) > startOffset &&
			strings.Contains(string(data[startOffset:]), "Starting bookmark sync") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
