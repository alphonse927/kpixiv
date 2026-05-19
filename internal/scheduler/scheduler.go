package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/cache"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

type Scheduler struct {
	cfg           *config.Config
	storage       *storage.Storage
	cache         *cache.Cache
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

func New(cfg *config.Config, st *storage.Storage, c *cache.Cache, p pixiv.ImageClient, s wallpaper.Setter) *Scheduler {
	return &Scheduler{
		cfg:           cfg,
		storage:       st,
		cache:         c,
		pixiv:         p,
		setter:        s,
		page:          1,
		setInterval:   time.Duration(cfg.Wallpaper.SetInterval) * time.Minute,
		fetchInterval: time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute,
		stopCh:        make(chan struct{}),
	}
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

	nextPage, err := sch.cache.Fetch(ctx, sch.pixiv, sch.cfg.Pixiv.Ranking.String(), sch.page, sch.cfg.Pixiv.R18)
	if err != nil {
		log.Error("Failed to fetch images", "error", err)
	} else {
		sch.page = nextPage
		log.Debug("Advanced ranking page", "nextPage", sch.page)
	}
}

func (sch *Scheduler) rotateWallpaper() {
	log := logger.WithComponent("scheduler")

	images := sch.cache.GetFiltered(sch.cfg.Pixiv.MinWidth, sch.cfg.Pixiv.MinHeight, sch.cfg.Pixiv.LandscapeOnly)
	if len(images) == 0 {
		log.Warn("No images available in cache")
		return
	}

	nextID, err := sch.storage.GetNextWallpaper()
	if err != nil {
		log.Error("Failed to get next wallpaper from history", "error", err)
		return
	}

	if nextID == "" {
		log.Info("No wallpaper in history, setting first available")
		nextID = images[0].ID
	}

	path, ok := sch.storage.GetImagePath(nextID)
	if !ok {
		log.Warn("Wallpaper not found in storage metadata", "id", nextID)

		for _, img := range images {
			candidatePath, candidateOK := sch.storage.GetImagePath(img.ID)
			if candidateOK {
				nextID = img.ID
				path = candidatePath
				ok = true
				break
			}
		}

		if !ok {
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
		log.Debug("Queue empty, loading available images")
		images, err := sch.storage.LoadMetadata()
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}

		valid := make([]string, 0, len(images))
		for id, meta := range images {
			if meta.Path != "" {
				valid = append(valid, id)
			}
		}

		if err = q.AppendRandom(valid); err != nil {
			return fmt.Errorf("failed to append images to queue: %w", err)
		}

		log.Debug("Loaded images into queue", "count", len(valid))
	}

	nextID, ok := q.Pop()
	if !ok {
		return fmt.Errorf("no wallpapers found in queue")
	}

	path, ok := sch.storage.GetImagePath(nextID)
	if !ok {
		log.Warn("Wallpaper not found in storage", "id", nextID)
		images, err := sch.storage.LoadMetadata()
		if err != nil {
			return fmt.Errorf("failed to load metadata: %w", err)
		}

		for id, meta := range images {
			if meta.Path != "" {
				nextID = id
				path = meta.Path
				break
			}
		}

		if path == "" {
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
	log.Info("Wallpaper set successfully")

	if err = sch.storage.AddToHistory(targetID); err != nil {
		log.Warn("Failed to update history", "error", err)
	}

	log.Info("Applied wallpaper on startup", "path", path)
	return nil
}
