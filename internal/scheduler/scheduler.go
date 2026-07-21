package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/bookmarks"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/fetcher"
	"github.com/alphonse927/kpixiv/internal/logger"
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
	page                 int
	setInterval          time.Duration
	fetchInterval        time.Duration
	bookmarkSyncInterval time.Duration
	stopCh               chan struct{}
	resetSetCh           chan struct{}
	resetFetchCh         chan struct{}
	wg                   sync.WaitGroup
	mu                   sync.Mutex
	running              bool
}

// New creates a scheduler for wallpaper rotation and periodic fetching.
func New(cfg *config.Config, st *storage.Storage, p pixiv.ImageClient, s wallpaper.Setter) *Scheduler {
	sch := &Scheduler{
		cfg:                  cfg,
		storage:              st,
		pixiv:                p,
		setter:               s,
		page:                 1,
		setInterval:          time.Duration(cfg.Wallpaper.SetInterval) * time.Minute,
		fetchInterval:        time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute,
		bookmarkSyncInterval: time.Duration(cfg.Bookmarks.SyncInterval) * time.Minute,
		stopCh:               make(chan struct{}),
		resetSetCh:           make(chan struct{}, 1),
		resetFetchCh:         make(chan struct{}, 1),
	}

	pageKey := fmt.Sprintf("%s:%t", cfg.Pixiv.Ranking, cfg.Pixiv.R18)
	if page, err := st.GetRankingPage(pageKey); err == nil && page > 1 {
		sch.page = page
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

	bookmarkChan, cleanup := sch.newBookmarkTicker()
	defer cleanup()

	for {
		select {
		case <-sch.stopCh:
			logger.Info("Scheduler stopped")
			return
		case <-sch.resetSetCh:
			resetTicker(setTicker, sch.setInterval)
		case <-sch.resetFetchCh:
			resetTicker(fetchTicker, sch.fetchInterval)
		case <-setTicker.C:
			if sch.cfg.Wallpaper.RotationEnabled {
				log.Debug("Setting wallpaper")
				if err := sch.rotateWallpaper(cname); err != nil {
					log.Warn("Failed to set wallpaper", "error", err)
				}
			}
		case <-fetchTicker.C:
			if sch.cfg.Wallpaper.FetchEnabled {
				if err := sch.fetchImages(ctx, cname); err != nil {
					log.Warn("Fetch tick failed", "error", err)
				}
			}
		case <-bookmarkChan:
			if err := sch.syncBookmarks(ctx, cname); err != nil {
				log.Warn("Bookmark sync tick failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (sch *Scheduler) newBookmarkTicker() (<-chan time.Time, func()) {
	if !sch.cfg.Bookmarks.Enabled {
		return nil, func() {}
	}
	ticker := time.NewTicker(sch.bookmarkSyncInterval)
	return ticker.C, ticker.Stop
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

func (sch *Scheduler) fetchImages(ctx context.Context, cname string) error {
	log := logger.WithComponent(cname)
	if sch.pixiv == nil {
		return fmt.Errorf("pixiv client is not configured")
	}

	f := fetcher.NewFetcher(sch.cfg, sch.storage, sch.pixiv)
	f.SetPage(sch.page)

	result, err := f.Fetch(ctx)
	if err != nil {
		log.Error("Failed to fetch", "error", err)
		return err
	}

	sch.page = result.NextPage
	log.Debug("Advanced ranking page", "nextPage", sch.page, "downloaded", result.Downloaded, "filtered", result.Filtered)
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

func (sch *Scheduler) syncBookmarks(ctx context.Context, cname string) error {
	log := logger.WithComponent(cname)

	pixivClient, ok := sch.pixiv.(*pixiv.Client)
	if !ok || pixivClient == nil || !pixivClient.LoggedIn() {
		log.Debug("Skipping bookmark sync: not logged in")
		return nil
	}

	if !sch.cfg.Bookmarks.Enabled {
		log.Debug("Skipping bookmark sync: disabled in config")
		return nil
	}

	syncer := bookmarks.NewSyncer(sch.cfg, sch.storage, pixivClient)
	result, err := syncer.Sync(ctx)
	if err != nil {
		log.Error("Bookmark sync failed", "error", err)
		return err
	}

	log.Debug("Bookmark sync complete", "downloaded", result.Downloaded, "deleted", result.Deleted)
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

	if len(valid) == 0 {
		return fmt.Errorf("no wallpapers found in storage")
	}

	if err = q.AppendRandom(valid); err != nil {
		return fmt.Errorf("failed to append to queue: %w", err)
	}

	log.Debug("Loaded images into queue", "count", len(valid))
	return nil
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

// ApplyConfig updates the scheduler's config and resets both tickers so new
// intervals take effect without stopping the scheduler goroutine.
func (sch *Scheduler) ApplyConfig(cfg *config.Config) {
	sch.mu.Lock()
	sch.cfg = cfg
	sch.setInterval = time.Duration(cfg.Wallpaper.SetInterval) * time.Minute
	sch.fetchInterval = time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute
	sch.bookmarkSyncInterval = time.Duration(cfg.Bookmarks.SyncInterval) * time.Minute
	running := sch.running
	sch.mu.Unlock()

	if !running {
		return
	}

	select {
	case sch.resetSetCh <- struct{}{}:
	default:
	}
	select {
	case sch.resetFetchCh <- struct{}{}:
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
	log := logger.WithComponent(cname)
	blacklist, err := sch.storage.LoadBlacklistSet()
	if err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}

	attempts := 0
	maxAttempts := 5
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

		path, exists := sch.storage.GetImagePath(nextID)
		if !exists {
			log.Warn("Wallpaper not found in storage, skipping...", "id", nextID)
			continue
		}

		if err = sch.setter.Set(path); err != nil {
			return fmt.Errorf("failed to set wallpaper: %w", err)
		}

		if err = sch.storage.AddToHistoryWithLimit(nextID, sch.cfg.Wallpaper.HistoryLimit); err != nil {
			return fmt.Errorf("failed to update history: %w", err)
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

// IsRunning reports whether the scheduler is currently active.
func (sch *Scheduler) IsRunning() bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.running
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

func selectTargetWallpaperID(images map[string]*storage.ImageMeta, blacklist map[string]struct{}, currentID, nextID string) string {
	for _, candidateID := range []string{currentID, nextID} {
		if wallpaperAvailable(images, blacklist, candidateID) {
			return candidateID
		}
	}

	for id, meta := range images {
		if _, excluded := blacklist[id]; excluded {
			continue
		}
		if meta.Path != "" {
			return id
		}
	}

	return ""
}

func wallpaperAvailable(images map[string]*storage.ImageMeta, blacklist map[string]struct{}, id string) bool {
	if id == "" {
		return false
	}

	if _, excluded := blacklist[id]; excluded {
		return false
	}

	_, ok := images[id]
	return ok
}

func (sch *Scheduler) rotateWallpaper(cname string) error {
	q := storage.NewQueue(sch.storage.StateDir())
	if err := q.Load(); err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}

	return sch.SetNextWallpaper(q, cname)
}
