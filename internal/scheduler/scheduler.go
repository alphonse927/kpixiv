package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kpixiv/kpixiv/internal/cache"
	"github.com/kpixiv/kpixiv/internal/config"
	"github.com/kpixiv/kpixiv/internal/logger"
	"github.com/kpixiv/kpixiv/internal/pixiv"
	"github.com/kpixiv/kpixiv/internal/storage"
	"github.com/kpixiv/kpixiv/internal/wallpaper"
)

type Scheduler struct {
	cfg      *config.Config
	storage  *storage.Storage
	cache    *cache.Cache
	pixiv    pixiv.PixivImageClient
	setter   wallpaper.Setter
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
		if err := sch.cache.Fetch(ctx, sch.pixiv, pixiv.RankingType(sch.cfg.Pixiv.Ranking), 1, sch.cfg.Pixiv.R18); err != nil {
			log.Error("Failed to fetch images", "error", err)
		}
	}

	images := sch.cache.GetFiltered(sch.cfg.Pixiv.MinWidth, sch.cfg.Pixiv.MinHeight, sch.cfg.Pixiv.LandscapeOnly)
	if len(images) == 0 {
		log.Warn("No images available in cache")
		return
	}

	nextPath, err := sch.storage.GetNextWallpaper()
	if err != nil {
		log.Error("Failed to get next wallpaper from history", "error", err)
		return
	}

	if nextPath == "" {
		log.Info("No wallpaper in history, setting first available")
		nextPath = images[0].ID
	}

	path, ok := sch.storage.GetImagePath(nextPath)
	if !ok {
		log.Warn("Wallpaper not found in storage", "id", nextPath)
		if len(images) > 0 {
			path = images[0].ID
		}
	}

	if err := sch.setter.Set(path); err != nil {
		log.Error("Failed to set wallpaper", "error", err)
		return
	}

	if err := sch.storage.AddToHistory(nextPath); err != nil {
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

	nextPath, err := sch.storage.GetNextWallpaper()
	if err != nil {
		return fmt.Errorf("failed to get next wallpaper: %w", err)
	}

	if nextPath == "" {
		return fmt.Errorf("no wallpaper in history")
	}

	if err := sch.setter.Set(nextPath); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	log.Info("Manually set next wallpaper", "path", nextPath)
	return nil
}

func (sch *Scheduler) IsRunning() bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	return sch.running
}
