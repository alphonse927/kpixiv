package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/notify"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
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
	if err := os.Setenv("HOME", tmpHome); err != nil {
		panic(err)
	}
	logger.Init(false)
	code := m.Run()
	_ = os.RemoveAll(tmpHome)
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

// screenApply records one SetForScreen call.
type screenApply struct {
	index string
	path  string
}

// mockMonitorSetter is a wallpaper.MonitorSetter with fixed screens.
type mockMonitorSetter struct {
	mockSetter
	screens    []wallpaper.Screen
	screensErr error

	mu      sync.Mutex
	applied []screenApply
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

func writeTestImages(t *testing.T, dir, prefix string, count int) []string {
	t.Helper()

	ids := make([]string, 0, count)
	for i := range count {
		id := fmt.Sprintf("%s%d", prefix, i)
		if err := os.WriteFile(filepath.Join(dir, id+".jpg"), []byte("img"), 0600); err != nil {
			t.Fatalf("WriteFile(%s) returned error: %v", id, err)
		}
		ids = append(ids, id)
	}

	return ids
}

func countIn(items []string, ids []string) int {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}

	n := 0
	for _, item := range items {
		if set[item] {
			n++
		}
	}

	return n
}

func TestRefillQueueAllLimitsBookmarksAndKeepsEveryRankingImage(t *testing.T) {
	cfg := testConfig()
	cfg.Wallpaper.QueueSource = config.QueueSourceAll
	s := testStorage(t)
	ranking := writeTestImages(t, s.RankingDir(), "rk-", 30)
	bookmarks := writeTestImages(t, s.BookmarksDir(), "bm-", 200)

	sch := New(cfg, s, &mockPixivClient{}, &mockSetter{})
	q := storage.NewQueue(s.StateDir())
	if err := sch.refillQueueFromStorage(q, "test"); err != nil {
		t.Fatalf("refillQueueFromStorage() returned error: %v", err)
	}

	items := q.GetAll()
	if got := countIn(items, ranking); got != len(ranking) {
		t.Errorf("ranking images in queue: got %d, want %d", got, len(ranking))
	}
	if got := countIn(items, bookmarks); got != maxBookmarksPerAllRefill {
		t.Errorf("bookmark images in queue: got %d, want %d", got, maxBookmarksPerAllRefill)
	}
	if want := len(ranking) + maxBookmarksPerAllRefill; len(items) != want {
		t.Errorf("queue length: got %d, want %d", len(items), want)
	}
}

func TestRefillQueueAllWithoutRankingStillCapsBookmarks(t *testing.T) {
	cfg := testConfig()
	cfg.Wallpaper.QueueSource = config.QueueSourceAll
	s := testStorage(t)
	bookmarks := writeTestImages(t, s.BookmarksDir(), "bm-", 200)

	sch := New(cfg, s, &mockPixivClient{}, &mockSetter{})
	q := storage.NewQueue(s.StateDir())
	if err := sch.refillQueueFromStorage(q, "test"); err != nil {
		t.Fatalf("refillQueueFromStorage() returned error: %v", err)
	}

	// Nothing has been downloaded yet, but the cap still applies so a large
	// bookmark library cannot fill the whole queue.
	items := q.GetAll()
	if len(items) != maxBookmarksPerAllRefill {
		t.Fatalf("queue length: got %d, want %d", len(items), maxBookmarksPerAllRefill)
	}
	if got := countIn(items, bookmarks); got != len(items) {
		t.Errorf("distinct bookmarks in queue: got %d, want %d", got, len(items))
	}
}

func TestRefillQueueAllWhenOrientationLeavesNoRankingStillCapsBookmarks(t *testing.T) {
	cfg := testConfig()
	cfg.Wallpaper.QueueSource = config.QueueSourceAll
	s := testStorage(t)
	ranking := writeTestImages(t, s.RankingDir(), "rk-", 3)
	bookmarks := writeTestImages(t, s.BookmarksDir(), "bm-", 25)

	meta := make(map[string]*storage.ImageMeta)
	for _, id := range ranking {
		meta[id] = &storage.ImageMeta{ID: id, Width: 1600, Height: 1000}
	}
	for _, id := range bookmarks {
		meta[id] = &storage.ImageMeta{ID: id, Width: 1000, Height: 1600}
	}
	if err := s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	sch := New(cfg, s, &mockPixivClient{}, &mockSetter{})
	mq := storage.NewMonitorQueue(s.StateDir(), "DP-1")
	if err := mq.SetOrientation(config.WallpaperPortraitOrientation.String()); err != nil {
		t.Fatalf("SetOrientation() returned error: %v", err)
	}
	if err := sch.refillQueueFromStorage(mq, "test"); err != nil {
		t.Fatalf("refillQueueFromStorage() returned error: %v", err)
	}

	// The portrait monitor can't use any ranking image, but the cap still
	// applies; every sampled bookmark must be usable by this queue.
	if got := mq.Len(); got != maxBookmarksPerAllRefill {
		t.Fatalf("queue length: got %d, want %d", got, maxBookmarksPerAllRefill)
	}
	items := mq.GetAll()
	if got := countIn(items, bookmarks); got != len(items) {
		t.Errorf("bookmarks in queue: got %d, want %d", got, len(items))
	}
}

func TestRefillQueueAllWithFewBookmarksQueuesAllOfThem(t *testing.T) {
	cfg := testConfig()
	cfg.Wallpaper.QueueSource = config.QueueSourceAll
	s := testStorage(t)
	ranking := writeTestImages(t, s.RankingDir(), "rk-", 5)
	writeTestImages(t, s.BookmarksDir(), "bm-", 4)

	sch := New(cfg, s, &mockPixivClient{}, &mockSetter{})
	q := storage.NewQueue(s.StateDir())
	if err := sch.refillQueueFromStorage(q, "test"); err != nil {
		t.Fatalf("refillQueueFromStorage() returned error: %v", err)
	}

	items := q.GetAll()
	if len(items) != len(ranking)+4 {
		t.Errorf("queue length: got %d, want %d", len(items), len(ranking)+4)
	}
}

func TestRefillQueueBookmarksSourceIsNotLimited(t *testing.T) {
	cfg := testConfig()
	cfg.Wallpaper.QueueSource = config.QueueSourceBookmarks
	s := testStorage(t)
	writeTestImages(t, s.RankingDir(), "rk-", 5)
	writeTestImages(t, s.BookmarksDir(), "bm-", 200)

	sch := New(cfg, s, &mockPixivClient{}, &mockSetter{})
	q := storage.NewQueue(s.StateDir())
	if err := sch.refillQueueFromStorage(q, "test"); err != nil {
		t.Fatalf("refillQueueFromStorage() returned error: %v", err)
	}

	if got := q.Len(); got != 200 {
		t.Errorf("queue length: got %d, want 200", got)
	}
}

func TestRefillQueueAllSamplesBookmarksAfterOrientationFilter(t *testing.T) {
	cfg := testConfig()
	cfg.Wallpaper.QueueSource = config.QueueSourceAll
	s := testStorage(t)
	portrait := writeTestImages(t, s.BookmarksDir(), "bm-p", 20)
	landscape := writeTestImages(t, s.BookmarksDir(), "bm-l", 10)

	meta := make(map[string]*storage.ImageMeta)
	for _, id := range portrait {
		meta[id] = &storage.ImageMeta{ID: id, Width: 1000, Height: 1600}
	}
	for _, id := range landscape {
		meta[id] = &storage.ImageMeta{ID: id, Width: 1600, Height: 1000}
	}
	if err := s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	sch := New(cfg, s, &mockPixivClient{}, &mockSetter{})
	mq := storage.NewMonitorQueue(s.StateDir(), "DP-1")
	if err := mq.SetOrientation(config.WallpaperPortraitOrientation.String()); err != nil {
		t.Fatalf("SetOrientation() returned error: %v", err)
	}
	if err := sch.refillQueueFromStorage(mq, "test"); err != nil {
		t.Fatalf("refillQueueFromStorage() returned error: %v", err)
	}

	// Sampling before filtering would leave about 6 or 7 portrait images
	// here; sampling after filtering fills all 10 slots with usable ones.
	items := mq.GetAll()
	if len(items) != maxBookmarksPerAllRefill {
		t.Fatalf("queue length: got %d, want %d", len(items), maxBookmarksPerAllRefill)
	}
	if got := countIn(items, portrait); got != len(items) {
		t.Errorf("portrait bookmarks in queue: got %d, want %d", got, len(items))
	}
}

func (m *mockMonitorSetter) Screens() ([]wallpaper.Screen, error) {
	return m.screens, m.screensErr
}

func (m *mockMonitorSetter) SetForScreen(index, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.applied = append(m.applied, screenApply{index: index, path: path})

	return nil
}

func (m *mockMonitorSetter) appliedIndexes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	indexes := make([]string, 0, len(m.applied))
	for _, a := range m.applied {
		indexes = append(indexes, a.index)
	}
	sort.Strings(indexes)

	return indexes
}

func twoScreens() []wallpaper.Screen {
	return []wallpaper.Screen{
		{ID: "DP-1", Index: "0"},
		{ID: "HDMI-1", Index: "1"},
	}
}

func multiMonitorConfig() *config.Config {
	cfg := testConfig()
	cfg.Wallpaper.MultiMonitorEnabled = true

	return cfg
}

// queueItems loads one queue from disk; an empty monitorID means the global queue.
func queueItems(t *testing.T, s *storage.Storage, monitorID string) []string {
	t.Helper()

	q := storage.NewQueue(s.StateDir())
	if monitorID != "" {
		q = storage.NewMonitorQueue(s.StateDir(), monitorID)
	}
	if err := q.Load(); err != nil {
		t.Fatalf("Load(%q) returned error: %v", monitorID, err)
	}

	return q.GetAll()
}

func sorted(items []string) []string {
	out := slices.Clone(items)
	sort.Strings(out)

	return out
}

func TestRebuildQueueMultiMonitorFillsMonitorQueuesOnly(t *testing.T) {
	s := testStorage(t)
	ids := writeTestImages(t, s.RankingDir(), "img-", 5)
	setter := &mockMonitorSetter{screens: twoScreens()}
	sch := New(multiMonitorConfig(), s, &mockPixivClient{}, setter)

	// A fresh install has no queue file yet.
	if _, err := os.Stat(filepath.Join(s.StateDir(), "queue.json")); !os.IsNotExist(err) {
		t.Fatalf("queue.json should not exist before the rebuild, stat error: %v", err)
	}

	if err := sch.RebuildQueue(); err != nil {
		t.Fatalf("RebuildQueue() returned error: %v", err)
	}

	for _, screen := range twoScreens() {
		if got := sorted(queueItems(t, s, screen.ID)); !slices.Equal(got, sorted(ids)) {
			t.Errorf("queue of %s: got %v, want %v", screen.ID, got, sorted(ids))
		}
	}
	if got := queueItems(t, s, ""); len(got) != 0 {
		t.Errorf("global queue should stay empty in multi-monitor mode, got %v", got)
	}
}

func TestRebuildQueueSingleMonitorFillsOnlyGlobalQueue(t *testing.T) {
	s := testStorage(t)
	ids := writeTestImages(t, s.RankingDir(), "img-", 5)
	// The setter can address screens, but the setting is off.
	setter := &mockMonitorSetter{screens: twoScreens()}
	sch := New(testConfig(), s, &mockPixivClient{}, setter)

	if err := sch.RebuildQueue(); err != nil {
		t.Fatalf("RebuildQueue() returned error: %v", err)
	}

	if got := sorted(queueItems(t, s, "")); !slices.Equal(got, sorted(ids)) {
		t.Errorf("global queue: got %v, want %v", got, sorted(ids))
	}
	for _, screen := range twoScreens() {
		if got := queueItems(t, s, screen.ID); len(got) != 0 {
			t.Errorf("queue of %s should stay empty in single-monitor mode, got %v", screen.ID, got)
		}
	}
}

func TestRebuildQueueMultiMonitorFallsBackToGlobalQueue(t *testing.T) {
	cases := map[string]wallpaper.Setter{
		"setter cannot address screens": &mockSetter{},
		"no screens reported":           &mockMonitorSetter{},
		"screens cannot be listed":      &mockMonitorSetter{screensErr: errors.New("dbus unavailable")},
	}

	for name, setter := range cases {
		t.Run(name, func(t *testing.T) {
			s := testStorage(t)
			ids := writeTestImages(t, s.RankingDir(), "img-", 3)
			sch := New(multiMonitorConfig(), s, &mockPixivClient{}, setter)

			if err := sch.RebuildQueue(); err != nil {
				t.Fatalf("RebuildQueue() returned error: %v", err)
			}

			if got := sorted(queueItems(t, s, "")); !slices.Equal(got, sorted(ids)) {
				t.Errorf("global queue: got %v, want %v", got, sorted(ids))
			}
		})
	}
}

func TestRebuildQueueWithoutImagesIsNotAnError(t *testing.T) {
	for name, cfg := range map[string]*config.Config{"single": testConfig(), "multi": multiMonitorConfig()} {
		t.Run(name, func(t *testing.T) {
			s := testStorage(t)
			sch := New(cfg, s, &mockPixivClient{}, &mockMonitorSetter{screens: twoScreens()})

			if err := sch.RebuildQueue(); err != nil {
				t.Fatalf("RebuildQueue() returned error: %v", err)
			}
		})
	}
}

func TestRebuildQueueReplacesStaleItems(t *testing.T) {
	s := testStorage(t)
	ids := writeTestImages(t, s.RankingDir(), "img-", 2)
	stale := storage.NewMonitorQueue(s.StateDir(), "DP-1")
	if err := stale.AppendRandom([]string{"stale"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	sch := New(multiMonitorConfig(), s, &mockPixivClient{}, &mockMonitorSetter{screens: twoScreens()})
	if err := sch.RebuildQueue(); err != nil {
		t.Fatalf("RebuildQueue() returned error: %v", err)
	}

	if got := sorted(queueItems(t, s, "DP-1")); !slices.Equal(got, sorted(ids)) {
		t.Errorf("queue of DP-1: got %v, want %v", got, sorted(ids))
	}
}

// saveOrientedImages writes one landscape and one portrait image and their metadata.
func saveOrientedImages(t *testing.T, s *storage.Storage) (landscape, portrait string) {
	t.Helper()

	writeTestImages(t, s.RankingDir(), "land-", 1)
	writeTestImages(t, s.RankingDir(), "port-", 1)
	meta := map[string]*storage.ImageMeta{
		"land-0": {ID: "land-0", Path: filepath.Join(s.RankingDir(), "land-0.jpg"), Width: 1920, Height: 1080},
		"port-0": {ID: "port-0", Path: filepath.Join(s.RankingDir(), "port-0.jpg"), Width: 1080, Height: 1920},
	}
	if err := s.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata() returned error: %v", err)
	}

	return "land-0", "port-0"
}

func orientedMonitorConfig() *config.Config {
	cfg := multiMonitorConfig()
	cfg.Wallpaper.Monitors = map[string]config.MonitorConfig{
		"DP-1":   {RotationEnabled: true, Orientation: config.WallpaperLandscapeOrientation},
		"HDMI-1": {RotationEnabled: true, Orientation: config.WallpaperPortraitOrientation},
	}

	return cfg
}

func TestRebuildQueueFiltersEachMonitorByItsOrientation(t *testing.T) {
	s := testStorage(t)
	landscape, portrait := saveOrientedImages(t, s)
	sch := New(orientedMonitorConfig(), s, &mockPixivClient{}, &mockMonitorSetter{screens: twoScreens()})

	if err := sch.RebuildQueue(); err != nil {
		t.Fatalf("RebuildQueue() returned error: %v", err)
	}

	if got := queueItems(t, s, "DP-1"); !slices.Equal(got, []string{landscape}) {
		t.Errorf("landscape monitor queue: got %v, want [%s]", got, landscape)
	}
	if got := queueItems(t, s, "HDMI-1"); !slices.Equal(got, []string{portrait}) {
		t.Errorf("portrait monitor queue: got %v, want [%s]", got, portrait)
	}
}

func TestEnqueueNewImagesFollowsMultiMonitorSetting(t *testing.T) {
	t.Run("multi-monitor feeds each screen by orientation", func(t *testing.T) {
		s := testStorage(t)
		landscape, portrait := saveOrientedImages(t, s)
		sch := New(orientedMonitorConfig(), s, &mockPixivClient{}, &mockMonitorSetter{screens: twoScreens()})

		if err := sch.enqueueNewImages([]string{landscape, portrait}); err != nil {
			t.Fatalf("enqueueNewImages() returned error: %v", err)
		}

		if got := queueItems(t, s, "DP-1"); !slices.Equal(got, []string{landscape}) {
			t.Errorf("landscape monitor queue: got %v, want [%s]", got, landscape)
		}
		if got := queueItems(t, s, "HDMI-1"); !slices.Equal(got, []string{portrait}) {
			t.Errorf("portrait monitor queue: got %v, want [%s]", got, portrait)
		}
		if got := queueItems(t, s, ""); len(got) != 0 {
			t.Errorf("global queue should stay empty, got %v", got)
		}
	})

	t.Run("single-monitor feeds the global queue", func(t *testing.T) {
		s := testStorage(t)
		sch := New(testConfig(), s, &mockPixivClient{}, &mockMonitorSetter{screens: twoScreens()})

		if err := sch.enqueueNewImages([]string{"a", "b"}); err != nil {
			t.Fatalf("enqueueNewImages() returned error: %v", err)
		}

		if got := sorted(queueItems(t, s, "")); !slices.Equal(got, []string{"a", "b"}) {
			t.Errorf("global queue: got %v, want [a b]", got)
		}
		if got := queueItems(t, s, "DP-1"); len(got) != 0 {
			t.Errorf("monitor queue should stay empty, got %v", got)
		}
	})
}

// writingPixivClient serves a fixed ranking and writes a file on download.
type writingPixivClient struct {
	images []pixiv.Image
}

func (c *writingPixivClient) FetchRanking(context.Context, string, int, bool) ([]pixiv.Image, int, error) {
	return c.images, 2, nil
}

func (c *writingPixivClient) DownloadImage(_ context.Context, _ *pixiv.Image, destPath string) error {
	return os.WriteFile(destPath, []byte("img"), 0600)
}

func TestFetchInMultiMonitorModeFeedsMonitorQueues(t *testing.T) {
	notify.SetEnabled(false)
	t.Cleanup(func() { notify.SetEnabled(true) })

	s := testStorage(t)
	client := &writingPixivClient{images: []pixiv.Image{
		{ID: "fresh-1", Width: 1920, Height: 1080},
		{ID: "fresh-2", Width: 2560, Height: 1440},
	}}
	sch := New(multiMonitorConfig(), s, client, &mockMonitorSetter{screens: twoScreens()})

	if err := sch.fetchImages(context.Background(), "test"); err != nil {
		t.Fatalf("fetchImages() returned error: %v", err)
	}

	for _, screen := range twoScreens() {
		if got := sorted(queueItems(t, s, screen.ID)); !slices.Equal(got, []string{"fresh-1", "fresh-2"}) {
			t.Errorf("queue of %s: got %v, want both fetched images", screen.ID, got)
		}
	}
	if got := queueItems(t, s, ""); len(got) != 0 {
		t.Errorf("global queue should stay empty in multi-monitor mode, got %v", got)
	}
}

func TestSetNextWallpaperForScreensOnlyAdvancesTheGivenScreens(t *testing.T) {
	s := testStorage(t)
	writeTestImages(t, s.RankingDir(), "img-", 6)
	setter := &mockMonitorSetter{screens: twoScreens()}
	cfg := multiMonitorConfig()
	cfg.Wallpaper.HistoryLimit = 10
	sch := New(cfg, s, &mockPixivClient{}, setter)

	if err := sch.SetNextWallpaperForScreens([]string{"HDMI-1"}, "test"); err != nil {
		t.Fatalf("SetNextWallpaperForScreens() returned error: %v", err)
	}

	if got := setter.appliedIndexes(); !slices.Equal(got, []string{"1"}) {
		t.Errorf("screens that received a wallpaper: got %v, want only [1] (HDMI-1)", got)
	}

	monitors, err := s.LoadMonitorHistory()
	if err != nil {
		t.Fatalf("LoadMonitorHistory() returned error: %v", err)
	}
	if monitors["HDMI-1"] == "" {
		t.Error("HDMI-1 should have a wallpaper recorded")
	}
	if _, touched := monitors["DP-1"]; touched {
		t.Errorf("DP-1 must not be touched, history: %v", monitors)
	}
}

func TestSetNextWallpaperForScreensIgnoresDisabledRotation(t *testing.T) {
	s := testStorage(t)
	writeTestImages(t, s.RankingDir(), "img-", 3)
	setter := &mockMonitorSetter{screens: twoScreens()}
	cfg := multiMonitorConfig()
	cfg.Wallpaper.HistoryLimit = 10
	cfg.Wallpaper.Monitors = map[string]config.MonitorConfig{"DP-1": {RotationEnabled: false}}
	sch := New(cfg, s, &mockPixivClient{}, setter)

	if err := sch.SetNextWallpaperForScreens([]string{"DP-1"}, "test"); err != nil {
		t.Fatalf("SetNextWallpaperForScreens() returned error: %v", err)
	}

	// The caller is replacing a wallpaper that is no longer usable, so the
	// screen's "rotation off" setting must not leave it on the old one.
	if got := setter.appliedIndexes(); !slices.Equal(got, []string{"0"}) {
		t.Errorf("screens that received a wallpaper: got %v, want [0]", got)
	}
}

func TestSetNextWallpaperForScreensFallsBackToGlobalWallpaper(t *testing.T) {
	s := testStorage(t)
	writeTestImages(t, s.RankingDir(), "img-", 3)
	setter := &mockSetter{}
	cfg := multiMonitorConfig()
	cfg.Wallpaper.HistoryLimit = 10
	sch := New(cfg, s, &mockPixivClient{}, setter)

	if err := sch.SetNextWallpaperForScreens([]string{"DP-1"}, "test"); err != nil {
		t.Fatalf("SetNextWallpaperForScreens() returned error: %v", err)
	}

	if !setter.setCalled {
		t.Error("without per-screen support the global wallpaper should be replaced")
	}
}
