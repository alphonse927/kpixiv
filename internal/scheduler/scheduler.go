package scheduler

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
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
	pixiv         pixiv.PixivImageClient
	setter        wallpaper.Setter
	page          int
	setInterval   time.Duration
	fetchInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	running       bool
}

func New(cfg *config.Config, st *storage.Storage, c *cache.Cache, p pixiv.PixivImageClient, s wallpaper.Setter) *Scheduler {
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
	log := logger.WithComponent("scheduler")
	log.Info("Stopping scheduler")

	close(sch.stopCh)
	sch.wg.Wait()

	sch.mu.Lock()
	sch.running = false
	sch.mu.Unlock()
}

func (sch *Scheduler) SetNext() error {
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

	n := len(valid)
	if n == 0 {
		return fmt.Errorf("no wallpapers found")
	}

	r, rErr := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if rErr != nil {
		return fmt.Errorf("failed to generate random index: %w", rErr)
	}

	nextID := valid[int(r.Int64())]
	path := images[nextID].Path

	if err = sch.setter.Set(path); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	if err = sch.storage.AddToHistory(nextID); err != nil {
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
