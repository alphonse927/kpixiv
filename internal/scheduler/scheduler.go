package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	cfg           *config.Config
	storage       *storage.Storage
	pixiv         pixiv.ImageClient
	setter        wallpaper.Setter
	page          int
	setInterval   time.Duration
	fetchInterval time.Duration
	stopCh        chan struct{}
	pauseCh       chan bool
	wg            sync.WaitGroup
	mu            sync.Mutex
	running       bool
}

// New creates a scheduler for wallpaper rotation and periodic fetching.
func New(cfg *config.Config, st *storage.Storage, p pixiv.ImageClient, s wallpaper.Setter) *Scheduler {
	sch := &Scheduler{
		cfg:           cfg,
		storage:       st,
		pixiv:         p,
		setter:        s,
		page:          1,
		setInterval:   time.Duration(cfg.Wallpaper.SetInterval) * time.Minute,
		fetchInterval: time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute,
		stopCh:        make(chan struct{}),
	}

	pageKey := fmt.Sprintf("%s:%t", cfg.Pixiv.Ranking, cfg.Pixiv.R18)
	if page, err := st.GetRankingPage(pageKey); err == nil && page > 1 {
		sch.page = page
	}

	sch.pauseCh = make(chan bool, 1)
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

	paused := false

	for {
		select {
		case <-sch.stopCh:
			logger.Info("Scheduler stopped")
			return
		case pause := <-sch.pauseCh:
			paused = pause
		case <-setTicker.C:
			if !paused {
				log.Debug("Setting wallpaper")
				if err := sch.rotateWallpaper(cname); err != nil {
					log.Warn("Failed to set wallpaper", "error", err)
				}
			}
		case <-fetchTicker.C:
			if err := sch.fetchImages(ctx, cname); err != nil {
				log.Warn("Fetch tick failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// Pause pauses scheduled wallpaper rotation.
func (sch *Scheduler) Pause() {
	select {
	case sch.pauseCh <- true:
	default:
	}
}

// Resume resumes scheduled wallpaper rotation.
func (sch *Scheduler) Resume() {
	select {
	case sch.pauseCh <- false:
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

func (sch *Scheduler) refillQueueFromRanking(q *storage.Queue, cname string) error {
	log := logger.WithComponent(cname)
	log.Debug("Queue empty, loading available images from Ranking folder")

	blacklist, err := sch.storage.LoadBlacklistSet()
	if err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}

	entries, err := os.ReadDir(sch.storage.RankingDir())
	if err != nil {
		return fmt.Errorf("failed to read ranking directory: %w", err)
	}

	valid := make([]string, 0)
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
		if _, excluded := blacklist[id]; excluded {
			continue
		}
		valid = append(valid, id)
	}

	if len(valid) == 0 {
		return fmt.Errorf("no wallpapers found in ranking folder")
	}

	if err = q.AppendRandom(valid); err != nil {
		return fmt.Errorf("failed to append to queue: %w", err)
	}

	log.Debug("Loaded images into queue", "count", len(valid))
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
			if err = sch.refillQueueFromRanking(q, cname); err != nil {
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
