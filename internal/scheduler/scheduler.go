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

type Scheduler struct {
	cfg           *config.Config
	storage       *storage.Storage
	pixiv         pixiv.ImageClient
	setter        wallpaper.Setter
	page          int
	setInterval   time.Duration
	fetchInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	running       bool
}

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

	return sch
}

func (sch *Scheduler) Run(ctx context.Context) error {
	sch.mu.Lock()
	if sch.running {
		sch.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	sch.running = true
	sch.mu.Unlock()

	log := logger.WithComponent("scheduler")
	log.Info("Starting scheduler", "setInterval", sch.setInterval, "fetchInterval", sch.fetchInterval)

	sch.wg.Add(1)
	go sch.run(ctx)

	return nil
}

func (sch *Scheduler) run(ctx context.Context) {
	defer sch.wg.Done()

	setTicker := time.NewTicker(sch.setInterval)
	defer setTicker.Stop()

	fetchTicker := time.NewTicker(sch.fetchInterval)
	defer fetchTicker.Stop()

	for {
		select {
		case <-sch.stopCh:
			logger.Info("Scheduler stopped")
			return
		case <-setTicker.C:
			sch.rotateWallpaper()
		case <-fetchTicker.C:
			sch.fetchImages(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (sch *Scheduler) fetchImages(ctx context.Context) {
	log := logger.WithComponent("scheduler")

	f := fetcher.NewFetcher(sch.cfg, sch.storage, sch.pixiv)
	f.SetPage(sch.page)

	result, err := f.Fetch(ctx)
	if err != nil {
		log.Error("Failed to fetch", "error", err)
		return
	}

	sch.page = result.NextPage
	log.Debug("Advanced ranking page", "nextPage", sch.page, "downloaded", result.Downloaded, "filtered", result.Filtered)
}

func (sch *Scheduler) rotateWallpaper() {
	log := logger.WithComponent("scheduler")

	q := storage.NewQueue(sch.storage.StateDir())
	if err := q.Load(); err != nil {
		log.Error("Failed to load queue", "error", err)
		return
	}

	if q.IsEmpty() {
		if err := sch.refillQueueFromRanking(q); err != nil {
			log.Warn("Failed to refill queue", "error", err)
			return
		}
	}

	nextID, hasNext := q.Pop()
	if !hasNext {
		log.Warn("No wallpapers found in queue")
		return
	}

	path, pathFound := sch.storage.GetImagePath(nextID)
	if !pathFound {
		nextID, path, pathFound = sch.findFallbackWallpaper()
		if !pathFound {
			log.Warn("No locally downloaded wallpapers available to apply")
			return
		}
	}

	if err := sch.setter.Set(path); err != nil {
		log.Error("Failed to set wallpaper", "error", err)
		return
	}

	if err := sch.storage.AddToHistory(nextID); err != nil {
		log.Error("Failed to update history", "error", err)
	}

	log.Info("Wallpaper set", "path", path)
}

func (sch *Scheduler) refillQueueFromRanking(q *storage.Queue) error {
	log := logger.WithComponent("scheduler")
	log.Debug("Queue empty, loading available images from Ranking folder")

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
		valid = append(valid, id)
	}

	if len(valid) == 0 {
		return fmt.Errorf("no wallpapers found in ranking folder")
	}

	if err := q.AppendRandom(valid); err != nil {
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

func (sch *Scheduler) findFallbackWallpaper() (string, string, bool) {
	log := logger.WithComponent("scheduler")
	log.Warn("Wallpaper not found in storage metadata, searching for fallback")

	entries, err := os.ReadDir(sch.storage.RankingDir())
	if err != nil {
		return "", "", false
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
		if p, found := sch.storage.GetImagePath(id); found {
			return id, p, true
		}
	}

	return "", "", false
}

func (sch *Scheduler) Stop() {
	sch.mu.Lock()
	if !sch.running {
		sch.mu.Unlock()
		return
	}
	sch.running = false
	sch.mu.Unlock()

	log := logger.WithComponent("scheduler")
	log.Info("Stopping scheduler")

	close(sch.stopCh)
	sch.wg.Wait()
}

func (sch *Scheduler) SetNext(q *storage.Queue) error {
	log := logger.WithComponent("scheduler")

	if q.IsEmpty() {
		log.Debug("Queue empty, loading available images from Ranking folder")

		if err := sch.refillQueueFromRanking(q); err != nil {
			return err
		}
	}

	nextID, ok := q.Pop()
	if !ok {
		return fmt.Errorf("no wallpapers found in queue")
	}

	path, ok := sch.storage.GetImagePath(nextID)
	if !ok {
		log.Warn("Wallpaper not found in storage", "id", nextID)

		nextID, path, ok = sch.findFallbackWallpaper()
		if !ok {
			return fmt.Errorf("no wallpapers available")
		}
	}

	if err := sch.setter.Set(path); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	if err := sch.storage.AddToHistory(nextID); err != nil {
		return fmt.Errorf("failed to update history: %w", err)
	}

	log.Info("Manually set next wallpaper", "path", path)
	return nil
}

func (sch *Scheduler) IsRunning() bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.running
}

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

	nextID, nwErr := sch.storage.GetNextWallpaper()
	if nwErr != nil {
		return fmt.Errorf("failed to get next wallpaper: %w", nwErr)
	}

	targetID := ""
	if currentID != "" {
		if _, ok := images[currentID]; ok {
			targetID = currentID
		}
	}
	if targetID == "" && nextID != "" {
		if _, ok := images[nextID]; ok {
			targetID = nextID
		}
	}
	if targetID == "" {
		for id, meta := range images {
			if meta.Path != "" {
				targetID = id
				break
			}
		}
	}

	if targetID == "" {
		return fmt.Errorf("no wallpapers available")
	}

	path := images[targetID].Path

	log.Info("Setting wallpaper", "path", path, "setter_type", fmt.Sprintf("%T", sch.setter))
	if err = sch.setter.Set(path); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	if err = sch.storage.AddToHistory(targetID); err != nil {
		log.Warn("Failed to update history", "error", err)
	}

	log.Info("Wallpaper set successfully", "path", path)
	return nil
}
