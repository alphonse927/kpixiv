package app

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/gui"
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
	pixiv  *pixiv.Client
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

	var client *pixiv.Client
	if !dryRun {
		client, err = pixiv.NewClient(st.StateDir())
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
		pixiv:  client,
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
		c.sch.Stop(componentName)
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	if len(images) == 0 {
		log := logger.WithComponent(componentName)
		log.Info("No downloaded images found, fetching initial wallpapers")

		if c.pixiv == nil {
			c.sch.Stop(componentName)
			return fmt.Errorf("no downloaded images found and pixiv client is unavailable")
		}

		// Fetch initial wallpapers
		if err = c.sch.FetchNowSync(c.ctx, componentName); err != nil {
			c.sch.Stop(componentName)
			return fmt.Errorf("failed initial fetch: %w", err)
		}

		// Apply wallpaper
		if err = c.sch.ApplyCurrentOrNext(); err != nil {
			c.sch.Stop(componentName)
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

	if err := c.sch.SetNextWallpaper(sq, trayComponentName); err != nil {
		return err
	}

	c.sch.ResetRotationTimer()
	return nil
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

// OpenCurrentArtworkInPixiv opens the current artwork's Pixiv page in the browser.
func (c *Controller) OpenCurrentArtworkInPixiv() error {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}
	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}

	url := fmt.Sprintf("https://www.pixiv.net/artworks/%s", currentID)
	logger.WithComponent(componentName).Debug("opening artwork in Pixiv", "id", currentID, "url", url)

	//nolint:gosec // url is constructed from a trusted internal ID.
	return exec.Command("xdg-open", url).Start()
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

	if err = c.sch.SetNextWallpaper(q, trayComponentName); err != nil {
		return err
	}

	c.sch.ResetRotationTimer()
	return nil
}

// PixivLoggedIn reports whether Pixiv account actions are available.
func (c *Controller) PixivLoggedIn() bool {
	return c.pixiv != nil && c.pixiv.LoggedIn()
}

// PixivUserName returns the logged-in Pixiv username when available.
func (c *Controller) PixivUserName() string {
	if c.pixiv == nil {
		return ""
	}

	return c.pixiv.AuthUserName()
}

// LoginToPixiv starts the Pixiv account login flow from the tray.
func (c *Controller) LoginToPixiv() error {
	if c.pixiv == nil {
		return fmt.Errorf("pixiv account actions are unavailable in dry-run mode")
	}

	flow, err := c.pixiv.BeginLogin()
	if err != nil {
		return err
	}

	if err = openExternal(flow.URL); err != nil {
		return fmt.Errorf("failed to open pixiv login page: %w", err)
	}

	instructions := "Pixiv login opened in your browser. After signing in, copy the final redirect URL from the address bar. Pixiv may end on either app-api callback URL or a pixiv://account/login URL. If the page errors, copy that full URL and paste it into the next dialog."
	if err = showInfoDialog("Pixiv Login", instructions); err != nil {
		return err
	}

	code, err := promptForInput("Pixiv Login", "Paste the Pixiv callback URL or code:")
	if err != nil {
		return err
	}

	_, err = c.pixiv.FinishLogin(c.ctx, flow.CodeVerifier, code)
	if err != nil {
		return err
	}

	return nil
}

// LogoutFromPixiv removes the stored Pixiv session.
func (c *Controller) LogoutFromPixiv() error {
	if c.pixiv == nil {
		return nil
	}

	if err := c.pixiv.Logout(); err != nil {
		return err
	}

	return nil
}

// CopyCurrentArtwork copies the current artwork into the configured download directory.
func (c *Controller) CopyCurrentArtwork() error {
	if !c.PixivLoggedIn() {
		return fmt.Errorf("pixiv login required")
	}

	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}
	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}

	logger.WithComponent(componentName).Debug("copying current artwork", "id", currentID)

	destPath, err := c.st.CopyImageToDownloadDir(currentID)
	if err != nil {
		return err
	}

	logger.WithComponent(componentName).Debug("artwork downloaded", "id", currentID, "dest", destPath)
	return nil
}

// BookmarkCurrentArtwork bookmarks the current artwork in Pixiv.
func (c *Controller) BookmarkCurrentArtwork() error {
	if !c.PixivLoggedIn() {
		return fmt.Errorf("pixiv login required")
	}

	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}
	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}

	logger.WithComponent(componentName).Debug("bookmarking current artwork on Pixiv", "id", currentID)

	if err = c.pixiv.BookmarkIllust(c.ctx, currentID); err != nil {
		return err
	}

	logger.WithComponent(componentName).Debug("artwork bookmarked on Pixiv, saving locally", "id", currentID)

	if err = c.st.AddBookmark(currentID); err != nil {
		logger.WithComponent(componentName).Warn("bookmarked on Pixiv but failed to save locally", "id", currentID, "error", err)
	}

	logger.WithComponent(componentName).Debug("artwork bookmark saved locally", "id", currentID)
	return nil
}

// IsArtworkBookmarked returns whether the current artwork has been locally bookmarked.
func (c *Controller) IsArtworkBookmarked() bool {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil || currentID == "" {
		return false
	}

	ok, err := c.st.IsArtworkBookmarked(currentID)
	if err != nil {
		return false
	}

	return ok
}

// Shutdown cancels running work and stops the scheduler.
func (c *Controller) Shutdown() {
	c.cancel()
	c.sch.Stop(componentName)
}

// ApplyConfig applies a new config to the scheduler without restarting it.
func (c *Controller) ApplyConfig(cfg *config.Config) {
	c.cfg = cfg
	if c.sch != nil {
		c.sch.ApplyConfig(cfg)
	}
}

// ShowSettingsWindow opens the settings window without blocking the tray.
func (c *Controller) ShowSettingsWindow() error {
	logger.WithComponent(componentName).Debug("Opening settings window")

	gui.ShowSettings(c.cfg, func() {
		c.ApplyConfig(c.cfg)
	})

	return nil
}

func openExternal(target string) error {
	return exec.Command("xdg-open", target).Start() //nolint:gosec // target is generated internally or a local path.
}
