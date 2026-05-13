package scheduler

import (
	"context"
	"fmt"
	"math/rand"
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
	cfg      *config.Config
	storage  *storage.Storage
	cache    *cache.Cache
	pixiv    pixiv.PixivImageClient
	setter   wallpaper.Setter
	page     int
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  bool
}

func New(cfg *config.Config, st *storage.Storage, c *cache.Cache, p pixiv.PixivImageClient, s wallpaper.Setter) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		storage:  st,
		cache:    c,
		pixiv:    p,
		setter:   s,
		page:     1,
		interval: time.Duration(cfg.IntervalMinutes) * time.Minute,
		stopCh:   make(chan struct{}),
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
	log.Info("Starting scheduler", "interval", sch.interval)

	sch.wg.Add(1)
	go sch.run(ctx)

	return nil
}

func (sch *Scheduler) run(ctx context.Context) {
	defer sch.wg.Done()

	ticker := time.NewTicker(sch.interval)
	defer ticker.Stop()

	sch.rotateWallpaper(ctx)

	for {
		select {
		case <-sch.stopCh:
			logger.Info("Scheduler stopped")
			return
		case <-ticker.C:
			sch.rotateWallpaper(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (sch *Scheduler) rotateWallpaper(ctx context.Context) {
	log := logger.WithComponent("scheduler")

	if sch.cache.NeedsFetch() {
		log.Debug("Cache needs refresh, fetching new images")
		nextPage, err := sch.cache.Fetch(ctx, sch.pixiv, pixiv.RankingType(sch.cfg.Pixiv.Ranking), sch.page, sch.cfg.Pixiv.R18)
		if err != nil {
			log.Error("Failed to fetch images", "error", err)
		} else {
			sch.page = nextPage
			log.Debug("Advanced ranking page", "nextPage", sch.page)
		}
	}

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
	log := logger.WithComponent("scheduler")
	log.Info("Stopping scheduler")

	close(sch.stopCh)
	sch.wg.Wait()

	sch.mu.Lock()
	sch.running = false
	sch.mu.Unlock()
}

func (sch *Scheduler) SetNext(ctx context.Context) error {
	log := logger.WithComponent("scheduler")

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

	if len(valid) == 0 {
		return fmt.Errorf("no wallpapers found")
	}

	r := rand.Intn(len(valid))
	nextID := valid[r]
	path := images[nextID].Path

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
