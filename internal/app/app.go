package app

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

const componentName = "app"

type Controller struct {
	cfg    *config.Config
	st     *storage.Storage
	sch    *scheduler.Scheduler
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	paused bool
}

const trayComponentName = "tray"

// New builds a controller with storage, wallpaper setter, and scheduler dependencies.
func New(cfg *config.Config, dryRun bool, reset bool) (*Controller, error) {
	st, err := storage.New("", cfg.DownloadPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	var client pixiv.ImageClient
	if !dryRun {
		client, err = pixiv.NewClient()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize pixiv client: %w", err)
		}
	}

	var setter wallpaper.Setter
	if dryRun {
		setter = wallpaper.NewDryRunSetter()
	} else {
		setter = wallpaper.NewKDESetter(cfg.KDE.SetLockScreen)
	}

	cleanupDays := cfg.Wallpaper.CleanupDays
	if reset {
		cleanupDays = 0
	}

	if removed, cleanupErr := st.CleanupImagesOlderThanDays(cleanupDays); cleanupErr != nil {
		logger.WithComponent(componentName).Warn("Failed to cleanup old images", "error", cleanupErr)
	} else {
		logger.WithComponent(componentName).Info("Image cleanup complete", "removed", removed, "days", cleanupDays)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Controller{
		cfg:    cfg,
		st:     st,
		sch:    scheduler.New(cfg, st, client, setter),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start starts the scheduler and applies or bootstraps the current wallpaper.
func (c *Controller) Start() error {
	if err := c.sch.Run(c.ctx); err != nil {
		return err
	}

	images, err := c.st.LoadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	if len(images) == 0 {
		log := logger.WithComponent(componentName)
		log.Info("No downloaded images found, fetching initial wallpapers")

		// Fetch initial wallpapers
		if err = c.sch.FetchNowSync(c.ctx, componentName); err != nil {
			return fmt.Errorf("failed initial fetch: %w", err)
		}

		// Apply wallpaper
		if err = c.sch.ApplyCurrentOrNext(); err != nil {
			return fmt.Errorf("failed to apply wallpaper after initial fetch: %w", err)
		}

		return nil
	}

	// Apply wallpaper
	if err = c.sch.ApplyCurrentOrNext(); err != nil {
		logger.WithComponent(componentName).Warn("Could not apply wallpaper on startup", "error", err)
	}

	return nil
}

// NextWallpaper applies the next wallpaper from the persisted queue.
func (c *Controller) NextWallpaper() error {
	sq := storage.NewQueue(c.st.StateDir())
	if err := sq.Load(); err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}

	return c.sch.SetNextWallpaper(sq, trayComponentName)
}

// PauseRotation pauses scheduled rotation and fetch ticks.
func (c *Controller) PauseRotation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused {
		return
	}
	c.paused = true
	c.sch.Pause()
}

// ResumeRotation resumes scheduled rotation and fetch ticks.
func (c *Controller) ResumeRotation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.paused {
		return
	}
	c.paused = false
	c.sch.Resume()
}

// OpenCurrentArtwork opens the currently active wallpaper in the system default app.
func (c *Controller) OpenCurrentArtwork() error {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}

	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}

	path, ok := c.st.GetImagePath(currentID)
	if !ok {
		return fmt.Errorf("current artwork not found on disk")
	}

	//nolint:gosec // xdg-open is intentionally used with a local filesystem path.
	return exec.Command("xdg-open", path).Start()
}

// ExcludeCurrentWallpaper blacklists the current wallpaper and switches to another one.
func (c *Controller) ExcludeCurrentWallpaper() error {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}

	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}

	if err = c.st.ExcludeWallpaper(currentID); err != nil {
		return fmt.Errorf("failed to exclude current artwork: %w", err)
	}

	q := storage.NewQueue(c.st.StateDir())
	if err = q.Load(); err != nil {
		return fmt.Errorf("excluded current artwork, but failed to load queue: %w", err)
	}

	if err = q.Remove(currentID); err != nil {
		return fmt.Errorf("excluded current artwork, but failed to update queue: %w", err)
	}

	return c.sch.SetNextWallpaper(q, trayComponentName)
}

// Shutdown cancels running work and stops the scheduler.
func (c *Controller) Shutdown() {
	c.cancel()
	c.sch.Stop(componentName)
}
