package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/bookmarks"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/fetcher"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/notify"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

const componentName = "scheduler"

// ErrImageNotFound indicates that a queued wallpaper ID does not exist in metadata.
var ErrImageNotFound = fmt.Errorf("image not found")

type Scheduler struct {
	cfg                  *config.Config
	storage              *storage.Storage
	pixiv                pixiv.ImageClient
	setter               wallpaper.Setter
	setInterval          time.Duration
	fetchInterval        time.Duration
	bookmarkSyncInterval time.Duration
	stopCh               chan struct{}
	resetSetCh           chan struct{}
	resetFetchCh         chan struct{}
	resetBookmarkCh      chan struct{}
	wg                   sync.WaitGroup
	mu                   sync.Mutex
	running              bool

	// Activity tracking for the "Fetching..."/"Syncing..." GUI indicators
	// and for computing an accurate "Next fetch"/"Next sync" countdown. The
	// "last attempt" fields update on every tick (success or failure),
	// unlike storage.Activity's LastFetchAt/LastBookmarkSyncAt, which only
	// record successful runs -- using only the success timestamp to predict
	// the next attempt meant a run of failures (auth expired, network down,
	// etc.) made the countdown get stuck at "Any moment now" forever, with
	// no visible indication that kPixiv was still trying every interval.
	fetchInProgress         bool
	lastFetchAttempt        time.Time
	lastFetchErr            error
	bookmarkSyncInProgress  bool
	lastBookmarkSyncAttempt time.Time
	lastBookmarkSyncErr     error
}

// New creates a scheduler for wallpaper rotation and periodic fetching.
func New(cfg *config.Config, st *storage.Storage, p pixiv.ImageClient, s wallpaper.Setter) *Scheduler {
	sch := &Scheduler{
		cfg:                  cfg,
		storage:              st,
		pixiv:                p,
		setter:               s,
		setInterval:          time.Duration(cfg.Wallpaper.SetInterval) * time.Minute,
		fetchInterval:        time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute,
		bookmarkSyncInterval: time.Duration(cfg.Bookmarks.SyncInterval) * time.Minute,
		stopCh:               make(chan struct{}),
		resetSetCh:           make(chan struct{}, 1),
		resetFetchCh:         make(chan struct{}, 1),
		resetBookmarkCh:      make(chan struct{}, 1),
	}

	return sch
}

// Run starts scheduler goroutines and ticker loops.
func (sch *Scheduler) Run(ctx context.Context) error {
	sch.mu.Lock()
	if sch.running {
		sch.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	sch.running = true
	sch.stopCh = make(chan struct{})
	sch.mu.Unlock()

	log := logger.WithComponent(componentName)
	log.Info("Starting scheduler", "setInterval", sch.setInterval, "fetchInterval", sch.fetchInterval)

	sch.wg.Add(1)
	go sch.run(ctx, componentName)

	return nil
}

func (sch *Scheduler) run(ctx context.Context, cname string) {
	log := logger.WithComponent(cname)
	defer sch.wg.Done()

	setTicker := time.NewTicker(sch.setInterval)
	defer setTicker.Stop()

	fetchTicker := time.NewTicker(sch.fetchInterval)
	defer fetchTicker.Stop()

	// bookmarkTicker is always created when its interval is valid, mirroring
	// setTicker/fetchTicker above. Whether a tick actually triggers a sync is
	// decided inside the case body by reading the live cfg.Bookmarks.Enabled
	// flag, not by whether bookmarks were enabled at the moment the scheduler
	// started. Previously this ticker was only created when Bookmarks.Enabled
	// was already true at startup; if the user enabled bookmark sync later via
	// Settings, the ticker channel stayed nil forever and bookmark sync would
	// never run again until the app was restarted (surfaced in the GUI as
	// "Next sync: Any moment now" stuck indefinitely).
	//
	// A non-positive interval gets no ticker rather than panicking:
	// time.NewTicker rejects such values, and while the config loader clamps
	// real values to at least DefaultBookmarksSync, a raw zero-value config
	// must not crash the scheduler goroutine.
	var bookmarkTicker *time.Ticker
	var bookmarkTickerC <-chan time.Time
	if sch.bookmarkSyncInterval > 0 {
		bookmarkTicker = time.NewTicker(sch.bookmarkSyncInterval)
		defer bookmarkTicker.Stop()
		bookmarkTickerC = bookmarkTicker.C
	}

	// taskCtx is cancelled when the scheduler is told to stop (or the
	// caller's context ends), so the startup tasks below are guaranteed to
	// wind down before run() returns. Stop() waits on wg, so this also means
	// the fire-and-forget startup tasks never keep writing to storage after
	// the scheduler has stopped -- a concern in tests where the storage dir
	// is torn down right after Stop().
	taskCtx, cancelTask := context.WithCancel(ctx)
	defer cancelTask()
	go func() {
		select {
		case <-sch.stopCh:
			cancelTask()
		case <-taskCtx.Done():
		}
	}()

	initialDone := sch.launchInitialTasks(taskCtx, cname, log)
	defer func() { <-initialDone }()

	for {
		select {
		case <-sch.stopCh:
			logger.Info("Scheduler stopped")
			return
		case <-sch.resetSetCh:
			resetTicker(setTicker, sch.setInterval)
		case <-sch.resetFetchCh:
			resetTicker(fetchTicker, sch.fetchInterval)
		case <-sch.resetBookmarkCh:
			if bookmarkTicker != nil {
				resetTicker(bookmarkTicker, sch.bookmarkSyncInterval)
			}
		case <-setTicker.C:
			sch.handleSetTick(cname, log)
		case <-fetchTicker.C:
			sch.handleFetchTick(ctx, cname, log)
		case <-bookmarkTickerC:
			sch.handleBookmarkTick(ctx, cname, log)
		case <-ctx.Done():
			return
		}
	}
}

// launchInitialTasks runs an initial fetch and bookmark sync right away
// instead of waiting a full interval for the first attempt. Previously,
// starting kPixiv (or logging into Pixiv) meant sitting at "Never" / a full
// interval's countdown until the first tick, even though the whole point of
// starting the app is usually to get fresh wallpapers and bookmarks without
// delay. Each runs in its own goroutine so a slow first attempt doesn't delay
// the ticker loop below from starting. The returned channel is closed once
// every spawned task has completed.
func (sch *Scheduler) launchInitialTasks(ctx context.Context, cname string, log *slog.Logger) <-chan struct{} {
	var tasks sync.WaitGroup
	if sch.cfg.Wallpaper.FetchEnabled {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			if err := sch.fetchImages(ctx, cname); err != nil {
				log.Warn("Initial fetch failed", "error", err)
			}
		}()
	}
	if sch.cfg.Bookmarks.Enabled {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			if err := sch.syncBookmarks(ctx, cname); err != nil {
				log.Warn("Initial bookmark sync failed", "error", err)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		tasks.Wait()
		close(done)
	}()
	return done
}

func (sch *Scheduler) handleSetTick(cname string, log *slog.Logger) {
	if !sch.cfg.Wallpaper.RotationEnabled && !sch.cfg.Wallpaper.MultiMonitorEnabled {
		return
	}
	log.Debug("Setting wallpaper")
	if err := sch.rotateWallpaper(cname); err != nil {
		log.Warn("Failed to set wallpaper", "error", err)
	}
}

func (sch *Scheduler) handleFetchTick(ctx context.Context, cname string, log *slog.Logger) {
	if !sch.cfg.Wallpaper.FetchEnabled {
		return
	}
	if err := sch.fetchImages(ctx, cname); err != nil {
		log.Warn("Fetch tick failed", "error", err)
	}
}

func (sch *Scheduler) handleBookmarkTick(ctx context.Context, cname string, log *slog.Logger) {
	if !sch.cfg.Bookmarks.Enabled {
		return
	}
	if err := sch.syncBookmarks(ctx, cname); err != nil {
		log.Warn("Bookmark sync tick failed", "error", err)
	}
}

func resetTicker(ticker *time.Ticker, interval time.Duration) {
	for {
		select {
		case <-ticker.C:
		default:
			ticker.Reset(interval)
			return
		}
	}
}

// ResetRotationTimer restarts the wallpaper rotation countdown from now.
func (sch *Scheduler) ResetRotationTimer() {
	sch.mu.Lock()
	running := sch.running
	sch.mu.Unlock()
	if !running {
		return
	}

	select {
	case sch.resetSetCh <- struct{}{}:
	default:
	}
}

// Restart stops and starts the scheduler, then triggers an immediate fetch.
func (sch *Scheduler) Restart(ctx context.Context, cname string) {
	sch.Stop(cname)
	if err := sch.Run(ctx); err != nil {
		logger.WithComponent("scheduler").Warn("Failed to restart scheduler", "error", err)
		return
	}
	sch.FetchNow(ctx, cname)
}

// FetchNow triggers an immediate image fetch for the provided component.
func (sch *Scheduler) FetchNow(ctx context.Context, cname string) {
	go func() {
		if err := sch.fetchImages(ctx, cname); err != nil {
			logger.WithComponent("scheduler").Debug("Background fetch failed", "error", err)
		}
	}()
}

// FetchNowSync performs a blocking fetch for the provided component.
func (sch *Scheduler) FetchNowSync(ctx context.Context, cname string) error {
	return sch.fetchImages(ctx, cname)
}

func (sch *Scheduler) fetchImages(ctx context.Context, cname string) (err error) {
	end := sch.beginFetch()
	defer func() { end(err) }()

	log := logger.WithComponent(cname)
	log.Info("Starting ranking fetch")
	if sch.pixiv == nil {
		err = fmt.Errorf("pixiv client is not configured")
		return err
	}

	f := fetcher.NewFetcher(sch.cfg, sch.storage, sch.pixiv)
	if err = f.LoadPage(); err != nil {
		log.Error("Failed to load ranking page", "error", err)
		return err
	}

	var result *fetcher.FetchResult
	result, err = f.Fetch(ctx)
	if err != nil {
		log.Error("Failed to fetch", "error", err)
		return err
	}

	log.Info("Ranking fetch complete", "downloaded", result.Downloaded, "filtered", result.Filtered, "nextPage", result.NextPage)
	if result.Downloaded > 0 {
		notify.SendDefault("KPixiv", fmt.Sprintf("Downloaded %d new wallpaper%s.", result.Downloaded, pluralize(result.Downloaded)))
	} else if result.Failed > 0 {
		notify.SendDefault("KPixiv", fmt.Sprintf("Wallpaper fetch had %d failure%s.", result.Failed, pluralize(result.Failed)))
	}
	return nil
}

// SyncBookmarksNow triggers an immediate bookmark sync in the background.
func (sch *Scheduler) SyncBookmarksNow(ctx context.Context, cname string) {
	go func() {
		if err := sch.syncBookmarks(ctx, cname); err != nil {
			logger.WithComponent("scheduler").Debug("Background bookmark sync failed", "error", err)
		}
	}()
}

// SyncBookmarksNowSync performs a blocking bookmark sync.
func (sch *Scheduler) SyncBookmarksNowSync(ctx context.Context, cname string) error {
	return sch.syncBookmarks(ctx, cname)
}

func (sch *Scheduler) syncBookmarks(ctx context.Context, cname string) (err error) {
	end := sch.beginBookmarkSync()
	defer func() { end(err) }()

	log := logger.WithComponent(cname)
	log.Info("Starting bookmark sync")

	pixivClient, ok := sch.pixiv.(*pixiv.Client)
	if !ok || pixivClient == nil || !pixivClient.LoggedIn() {
		log.Info("Skipping bookmark sync: not logged in")
		return nil
	}

	if !sch.cfg.Bookmarks.Enabled {
		log.Info("Skipping bookmark sync: disabled in config")
		return nil
	}

	syncer := bookmarks.NewSyncer(sch.cfg, sch.storage, pixivClient)
	var result *bookmarks.SyncResult
	result, err = syncer.Sync(ctx)
	if err != nil {
		log.Error("Bookmark sync failed", "error", err)
		if errors.Is(err, pixiv.ErrAuthSessionInvalid) {
			notify.SendDefault("KPixiv", "Pixiv login expired. Run 'kpixivctl account login' to reconnect.")
		}
		return err
	}

	log.Info("Bookmark sync complete", "downloaded", result.Downloaded, "deleted", result.Deleted)
	if result.Downloaded > 0 || result.Deleted > 0 {
		notify.SendDefault("KPixiv", fmt.Sprintf("Bookmark sync: downloaded %d, removed %d.", result.Downloaded, result.Deleted))
	}
	return nil
}

func (sch *Scheduler) refillQueueFromStorage(q *storage.Queue, cname string) error {
	log := logger.WithComponent(cname)
	queueSource := sch.cfg.Wallpaper.QueueSource
	log.Debug("Queue empty, loading available images", "source", queueSource)

	blacklist, err := sch.storage.LoadBlacklistSet()
	if err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}

	valid := make([]string, 0)
	seen := make(map[string]bool)

	switch queueSource {
	case config.QueueSourceBookmarks:
		err = sch.collectImagesFromDir(sch.storage.BookmarksDir(), blacklist, seen, &valid)
		if err != nil {
			return fmt.Errorf("failed to read favorites directory: %w", err)
		}
	case config.QueueSourceRanking:
		err = sch.collectImagesFromDir(sch.storage.RankingDir(), blacklist, seen, &valid)
		if err != nil {
			return fmt.Errorf("failed to read ranking directory: %w", err)
		}
	default:
		err = sch.collectImagesFromDir(sch.storage.RankingDir(), blacklist, seen, &valid)
		if err != nil {
			return fmt.Errorf("failed to read ranking directory: %w", err)
		}
		err = sch.collectImagesFromDir(sch.storage.BookmarksDir(), blacklist, seen, &valid)
		if err != nil {
			return fmt.Errorf("failed to read favorites directory: %w", err)
		}
	}

	valid, err = sch.filterQueueByOrientation(q, valid, blacklist)
	if err != nil {
		return err
	}

	if len(valid) == 0 {
		return fmt.Errorf("no wallpapers found in storage")
	}

	if err = q.AppendRandom(valid); err != nil {
		return fmt.Errorf("failed to append to queue: %w", err)
	}

	log.Debug("Loaded images into queue", "count", len(valid))
	return nil
}

func (sch *Scheduler) filterQueueByOrientation(q *storage.Queue, valid []string, blacklist map[string]struct{}) ([]string, error) {
	orientation := q.Orientation()
	if orientation == "" || orientation == config.WallpaperAnyOrientation.String() {
		return valid, nil
	}

	images, err := sch.storage.LoadMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	filtered := valid[:0]
	for _, id := range valid {
		if matchesOrientation(images[id], config.WallpaperOrientation(orientation)) {
			filtered = append(filtered, id)
		}
	}

	if len(filtered) > 0 {
		return filtered, nil
	}

	fallback := make([]string, 0)
	seen := make(map[string]bool)
	if err = sch.collectImagesFromDir(sch.storage.RankingDir(), blacklist, seen, &fallback); err != nil {
		return nil, fmt.Errorf("failed to read ranking directory: %w", err)
	}

	if err = sch.collectImagesFromDir(sch.storage.BookmarksDir(), blacklist, seen, &fallback); err != nil {
		return nil, fmt.Errorf("failed to read bookmarks directory: %w", err)
	}

	result := fallback[:0]
	for _, id := range fallback {
		if matchesOrientation(images[id], config.WallpaperOrientation(orientation)) {
			result = append(result, id)
		}
	}

	return result, nil
}

func (sch *Scheduler) collectImagesFromDir(dir string, blacklist map[string]struct{}, seen map[string]bool, valid *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isValidWallpaperFile(entry) {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		id := strings.TrimSuffix(name, ext)
		if seen[id] {
			continue
		}
		if _, excluded := blacklist[id]; excluded {
			continue
		}
		seen[id] = true
		*valid = append(*valid, id)
	}

	return nil
}

func isValidWallpaperFile(entry os.DirEntry) bool {
	name := entry.Name()
	ext := strings.ToLower(filepath.Ext(name))

	switch ext {
	case ".jpg", ".jpeg", ".png":
	default:
		return false
	}

	info, err := entry.Info()
	if err != nil {
		return false
	}

	if info.Size() == 0 {
		return false
	}

	return true
}

// ApplyConfig updates the scheduler's config and resets the set, fetch, and
// bookmark-sync tickers so new intervals (and newly-enabled bookmark sync)
// take effect without stopping the scheduler goroutine.
func (sch *Scheduler) ApplyConfig(cfg *config.Config) {
	sch.mu.Lock()
	sch.cfg = cfg
	sch.setInterval = time.Duration(cfg.Wallpaper.SetInterval) * time.Minute
	sch.fetchInterval = time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute
	sch.bookmarkSyncInterval = time.Duration(cfg.Bookmarks.SyncInterval) * time.Minute
	notify.SetEnabled(cfg.Notifications.Enabled)
	running := sch.running
	sch.mu.Unlock()

	if !running {
		return
	}

	if sch.cfg.Wallpaper.MultiMonitorEnabled {
		if err := sch.RebuildMonitorQueues(); err != nil {
			logger.WithComponent("scheduler").Warn("Failed to rebuild monitor queues", "error", err)
		}
	}

	select {
	case sch.resetSetCh <- struct{}{}:
	default:
	}
	select {
	case sch.resetFetchCh <- struct{}{}:
	default:
	}
	select {
	case sch.resetBookmarkCh <- struct{}{}:
	default:
	}
}

// Stop stops the scheduler goroutines and waits for completion.
func (sch *Scheduler) Stop(cname string) {
	sch.mu.Lock()
	if !sch.running {
		sch.mu.Unlock()
		return
	}
	sch.running = false
	sch.mu.Unlock()

	log := logger.WithComponent(cname)
	log.Info("Stopping scheduler")

	close(sch.stopCh)
	sch.wg.Wait()
}

// RebuildQueue clears the queue and refills it from available images.
func (sch *Scheduler) RebuildQueue() error {
	log := logger.WithComponent(componentName)

	q := storage.NewQueue(sch.storage.StateDir())
	if err := q.Load(); err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}

	if err := q.Clear(); err != nil {
		return fmt.Errorf("failed to clear queue: %w", err)
	}

	if err := sch.refillQueueFromStorage(q, componentName); err != nil {
		log.Debug("Queue rebuild: no images available", "error", err)
	}

	log.Debug("Queue rebuilt", "count", q.Len())
	return nil
}

// SetNextWallpaper applies the next wallpaper from the queue and updates history.
func (sch *Scheduler) SetNextWallpaper(q *storage.Queue, cname string) error {
	return sch.setNextWallpaper(q, "", "", cname)
}

// SetNextWallpaperForScreen advances the shared queue and applies the result
// only to the requested screen.
func (sch *Scheduler) SetNextWallpaperForScreen(_ *storage.Queue, screenID, screenIndex, cname string) error {
	mq := storage.NewMonitorQueue(sch.storage.StateDir(), screenID)
	if err := mq.Load(); err != nil {
		return err
	}

	if err := mq.SetOrientation(sch.monitorOrientation(screenID).String()); err != nil {
		return err
	}

	return sch.setNextWallpaper(mq, screenID, screenIndex, cname)
}

// SetNextWallpapers advances the rotation on every active screen.
func (sch *Scheduler) SetNextWallpapers(cname string) error {
	return sch.rotateWallpaper(cname)
}

// ResolveScreenIndex returns the Plasma screen index for a given connector name.
// Falls back to monitorID if it cannot be resolved.
func (sch *Scheduler) ResolveScreenIndex(monitorID string) string {
	screen := sch.resolveScreen(monitorID)
	if screen == nil {
		return monitorID
	}

	return screen.Index
}

// ResolveScreen returns the Screen for a connector name or numeric screen
// index. Returns nil when no monitor matches.
func (sch *Scheduler) ResolveScreen(monitorID string) *wallpaper.Screen {
	return sch.resolveScreen(monitorID)
}

// resolveScreen looks up a screen by connector name or numeric index.
func (sch *Scheduler) resolveScreen(monitorID string) *wallpaper.Screen {
	monitorSetter, ok := sch.setter.(wallpaper.MonitorSetter)
	if !ok {
		return nil
	}

	screens, err := monitorSetter.Screens()
	if err != nil {
		return nil
	}

	// First try matching by connector name (e.g. "DP-1").
	for _, s := range screens {
		if s.ID == monitorID {
			return &s
		}
	}

	// Then try matching by numeric screen index (e.g. "0").
	for _, s := range screens {
		if s.Index == monitorID {
			return &s
		}
	}

	return nil
}

func (sch *Scheduler) setNextWallpaper(q *storage.Queue, screenID, screenIndex, cname string) error {
	log := logger.WithComponent(cname)
	blacklist, err := sch.storage.LoadBlacklistSet()
	if err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}

	attempts := 0
	maxAttempts := 5
	orientation := sch.monitorOrientation(screenID)
	applied := false
	for attempts < maxAttempts {
		attempts++

		if q.IsEmpty() {
			if err = sch.refillQueueFromStorage(q, cname); err != nil {
				return err
			}

			if q.IsEmpty() {
				return ErrImageNotFound
			}
		}

		nextID, ok := q.Pop()
		if !ok {
			return fmt.Errorf("no wallpapers found in queue")
		}

		if _, excluded := blacklist[nextID]; excluded {
			log.Debug("Skipping blacklisted wallpaper", "id", nextID)
			continue
		}
		images, loadErr := sch.storage.LoadMetadata()
		if loadErr != nil {
			return fmt.Errorf("failed to load metadata: %w", loadErr)
		}
		if !matchesOrientation(images[nextID], orientation) {
			log.Debug("Skipping wallpaper with incompatible orientation", "id", nextID, "orientation", orientation)
			continue
		}

		path, exists := sch.storage.GetImagePath(nextID)
		if !exists {
			log.Warn("Wallpaper not found in storage, skipping...", "id", nextID)
			continue
		}

		if err = sch.applyWallpaperSet(screenIndex, path); err != nil {
			return fmt.Errorf("failed to set wallpaper: %w", err)
		}

		if err = sch.addWallpaperHistory(screenID, nextID); err != nil {
			return fmt.Errorf("failed to update wallpaper history: %w", err)
		}

		log.Info("New wallpaper set", "path", path)
		applied = true
		break
	}

	if !applied {
		return ErrImageNotFound
	}

	return nil
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (sch *Scheduler) applyWallpaperSet(screenIndex, path string) error {
	if screenIndex == "" {
		return sch.setter.Set(path)
	}

	monitorSetter, ok := sch.setter.(wallpaper.MonitorSetter)
	if !ok {
		return fmt.Errorf("wallpaper setter does not support screen index %q", screenIndex)
	}

	return monitorSetter.SetForScreen(screenIndex, path)
}

func (sch *Scheduler) addWallpaperHistory(screenID, nextID string) error {
	if screenID == "" {
		return sch.storage.AddToHistoryWithLimit(nextID, sch.cfg.Wallpaper.HistoryLimit)
	}

	return sch.storage.AddToMonitorHistory(screenID, nextID, sch.cfg.Wallpaper.HistoryLimit)
}

// IsRunning reports whether the scheduler is currently active.
func (sch *Scheduler) IsRunning() bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.running
}

// FetchInProgress reports whether a ranking fetch is actively running right now.
func (sch *Scheduler) FetchInProgress() bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.fetchInProgress
}

// LastFetchAttempt returns when the most recent fetch attempt started,
// regardless of whether it succeeded. Used to compute the "Next fetch"
// countdown, since basing it on the last *successful* fetch alone would get
// stuck once fetches start failing.
func (sch *Scheduler) LastFetchAttempt() time.Time {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.lastFetchAttempt
}

// LastFetchError returns the error from the most recent fetch attempt, or
// nil if it succeeded (or none has run yet).
func (sch *Scheduler) LastFetchError() error {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.lastFetchErr
}

// BookmarkSyncInProgress reports whether a bookmark sync is actively running right now.
func (sch *Scheduler) BookmarkSyncInProgress() bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.bookmarkSyncInProgress
}

// LastBookmarkSyncAttempt returns when the most recent bookmark sync attempt
// started, regardless of whether it succeeded. See LastFetchAttempt.
func (sch *Scheduler) LastBookmarkSyncAttempt() time.Time {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.lastBookmarkSyncAttempt
}

// LastBookmarkSyncError returns the error from the most recent bookmark sync
// attempt, or nil if it succeeded (or none has run yet).
func (sch *Scheduler) LastBookmarkSyncError() error {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.lastBookmarkSyncErr
}

// beginFetch marks a fetch attempt as started and returns a function to call
// with its result when done.
func (sch *Scheduler) beginFetch() func(error) {
	sch.mu.Lock()
	sch.fetchInProgress = true
	sch.lastFetchAttempt = time.Now()
	sch.mu.Unlock()

	return func(err error) {
		sch.mu.Lock()
		sch.fetchInProgress = false
		sch.lastFetchErr = err
		sch.mu.Unlock()
	}
}

// beginBookmarkSync marks a bookmark sync attempt as started and returns a
// function to call with its result when done.
func (sch *Scheduler) beginBookmarkSync() func(error) {
	sch.mu.Lock()
	sch.bookmarkSyncInProgress = true
	sch.lastBookmarkSyncAttempt = time.Now()
	sch.mu.Unlock()

	return func(err error) {
		sch.mu.Lock()
		sch.bookmarkSyncInProgress = false
		sch.lastBookmarkSyncErr = err
		sch.mu.Unlock()
	}
}

// ApplyCurrentOrNext applies the current wallpaper or a valid fallback.
func (sch *Scheduler) ApplyCurrentOrNext() error {
	log := logger.WithComponent("scheduler")

	images, err := sch.storage.LoadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	currentID, cwErr := sch.storage.GetCurrentWallpaper()
	if cwErr != nil {
		return fmt.Errorf("failed to get current wallpaper: %w", cwErr)
	}

	blacklist, err := sch.storage.LoadBlacklistSet()
	if err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}

	nextID, nwErr := sch.storage.GetNextWallpaper()
	if nwErr != nil {
		return fmt.Errorf("failed to get next wallpaper: %w", nwErr)
	}

	targetID := selectTargetWallpaperID(images, blacklist, currentID, nextID)
	if targetID == "" {
		return fmt.Errorf("no wallpapers available")
	}

	path := images[targetID].Path
	if sch.cfg.Wallpaper.MultiMonitorEnabled {
		applied, multiErr := sch.applyCurrentOrNextMultiMonitor(images, blacklist, targetID)
		if multiErr != nil {
			return multiErr
		}
		if applied {
			return nil
		}
	}

	log.Info("Setting wallpaper", "path", path, "setter_type", fmt.Sprintf("%T", sch.setter))
	if err = sch.setter.Set(path); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	if err = sch.storage.AddToHistoryWithLimit(targetID, sch.cfg.Wallpaper.HistoryLimit); err != nil {
		log.Warn("Failed to update history", "error", err)
	}

	log.Info("Wallpaper set successfully", "path", path)
	return nil
}

func (sch *Scheduler) applyCurrentOrNextMultiMonitor(images map[string]*storage.ImageMeta, blacklist map[string]struct{}, targetID string) (bool, error) {
	log := logger.WithComponent("scheduler")
	monitorSetter, ok := sch.setter.(wallpaper.MonitorSetter)
	if !ok {
		return false, nil
	}

	screens, err := monitorSetter.Screens()
	if err != nil {
		return false, nil //nolint:nilerr // intentional fall-through to single-monitor
	}

	if len(screens) == 0 {
		return false, nil
	}

	monitors, err := sch.storage.LoadMonitorHistory()
	if err != nil {
		return false, fmt.Errorf("failed to load monitor history: %w", err)
	}

	for _, screen := range screens {
		if multiErr := sch.applyScreenWallpaper(screen, images, blacklist, targetID, monitors, monitorSetter); multiErr != nil {
			return false, multiErr
		}
	}

	if err = sch.storage.SaveMonitorHistory(monitors, sch.cfg.Wallpaper.HistoryLimit); err != nil {
		return false, fmt.Errorf("failed to save monitor history: %w", err)
	}

	log.Info("Per-screen wallpapers restored", "screens", len(screens))
	return true, nil
}

func (sch *Scheduler) applyScreenWallpaper(screen wallpaper.Screen, images map[string]*storage.ImageMeta, blacklist map[string]struct{}, targetID string, monitors map[string]string, monitorSetter wallpaper.MonitorSetter) error {
	settings, configured := sch.cfg.Wallpaper.Monitors[screen.ID]
	if configured && !settings.RotationEnabled {
		return nil
	}

	orientation := config.WallpaperAnyOrientation
	if settings.Orientation != "" {
		orientation = settings.Orientation
	}

	monitorID := monitors[screen.ID]
	if !wallpaperAvailable(images, blacklist, monitorID) || images[monitorID].Path == "" || !matchesOrientation(images[monitorID], orientation) {
		monitorID = targetID
	}

	if !matchesOrientation(images[monitorID], orientation) {
		monitorID = selectTargetWallpaperIDForOrientation(images, blacklist, orientation, targetID)
		if monitorID == "" {
			return nil
		}
	}

	if err := monitorSetter.SetForScreen(screen.Index, images[monitorID].Path); err != nil {
		return fmt.Errorf("failed to restore wallpaper on screen %s: %w", screen.ID, err)
	}

	monitors[screen.ID] = monitorID
	return nil
}

func selectTargetWallpaperID(images map[string]*storage.ImageMeta, blacklist map[string]struct{}, currentID, nextID string) string {
	return selectTargetWallpaperIDForOrientation(images, blacklist, config.WallpaperAnyOrientation, currentID, nextID)
}

func selectTargetWallpaperIDForOrientation(images map[string]*storage.ImageMeta, blacklist map[string]struct{}, orientation config.WallpaperOrientation, candidates ...string) string {
	for _, candidateID := range candidates {
		if wallpaperAvailable(images, blacklist, candidateID) && matchesOrientation(images[candidateID], orientation) {
			return candidateID
		}
	}

	for id, meta := range images {
		if _, excluded := blacklist[id]; excluded {
			continue
		}
		if meta != nil && meta.Path != "" && matchesOrientation(meta, orientation) {
			return id
		}
	}

	return ""
}

func matchesOrientation(meta *storage.ImageMeta, orientation config.WallpaperOrientation) bool {
	if meta == nil || meta.Width <= 0 || meta.Height <= 0 {
		return orientation == config.WallpaperAnyOrientation || orientation == ""
	}

	return orientation.Matches(meta.Width, meta.Height)
}

func wallpaperAvailable(images map[string]*storage.ImageMeta, blacklist map[string]struct{}, id string) bool {
	if id == "" {
		return false
	}

	if _, excluded := blacklist[id]; excluded {
		return false
	}

	meta, ok := images[id]
	return ok && meta != nil && meta.Path != ""
}

func (sch *Scheduler) rotateWallpaper(cname string) error {
	q := storage.NewQueue(sch.storage.StateDir())
	if err := q.Load(); err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}

	if !sch.cfg.Wallpaper.MultiMonitorEnabled {
		return sch.SetNextWallpaper(q, cname)
	}

	monitorSetter, ok := sch.setter.(wallpaper.MonitorSetter)
	if !ok {
		return sch.SetNextWallpaper(q, cname)
	}

	screens, err := monitorSetter.Screens()
	if err != nil || len(screens) == 0 {
		return sch.SetNextWallpaper(q, cname)
	}

	for _, screen := range screens {
		settings, configured := sch.cfg.Wallpaper.Monitors[screen.ID]
		if configured && !settings.RotationEnabled {
			continue
		}

		if err = sch.SetNextWallpaperForScreen(q, screen.ID, screen.Index, cname); err != nil && !errors.Is(err, ErrImageNotFound) {
			return fmt.Errorf("screen %s: %w", screen.ID, err)
		}
	}

	return nil
}

// monitorOrientation returns the orientation filter for a rotation. The
// shared queue (single-monitor mode) uses the global wallpaper orientation;
// per-screen rotations use each monitor's own orientation setting.
func (sch *Scheduler) monitorOrientation(screenID string) config.WallpaperOrientation {
	if screenID == "" {
		return sch.cfg.Wallpaper.Orientation
	}
	if settings, ok := sch.cfg.Wallpaper.Monitors[screenID]; ok && settings.Orientation != "" {
		return settings.Orientation
	}
	return config.WallpaperAnyOrientation
}

// RebuildMonitorQueues recreates all monitor queues from the main queue.
// This is called when monitor config or rotation settings change, ensuring
// every monitor starts with a fresh queue.
func (sch *Scheduler) RebuildMonitorQueues() error {
	if !sch.cfg.Wallpaper.MultiMonitorEnabled {
		return nil
	}

	monitorSetter, ok := sch.setter.(wallpaper.MonitorSetter)
	if !ok {
		return nil
	}

	screens, err := monitorSetter.Screens()
	if err != nil {
		return err
	}

	// Build a set of current screen IDs to detect stale entries.
	currentIDs := make(map[string]struct{}, len(screens))
	for _, s := range screens {
		currentIDs[s.ID] = struct{}{}
	}

	// Load the main queue to check for stale monitor entries.
	mq := storage.NewQueue(sch.storage.StateDir())
	if err = mq.Load(); err != nil {
		return err
	}

	// Recreate each monitor queue: clear and refill from the main queue.
	for _, screen := range screens {
		q := storage.NewMonitorQueue(sch.storage.StateDir(), screen.ID)
		if err = q.Load(); err != nil {
			return err
		}

		if err = q.SetOrientation(sch.monitorOrientation(screen.ID).String()); err != nil {
			return err
		}

		// Always clear and refill to match current settings.
		if !q.IsEmpty() {
			if clearErr := q.Clear(); clearErr != nil {
				return fmt.Errorf("failed to clear monitor queue for screen %s: %w", screen.ID, clearErr)
			}
		}

		if err = sch.refillQueueFromStorage(q, "scheduler"); err != nil {
			return fmt.Errorf("screen %s: %w", screen.ID, err)
		}
	}

	return nil
}
