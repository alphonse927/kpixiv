package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/gui"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/platform"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

const componentName = "app"

type Controller struct {
	cfg          *config.Config
	st           *storage.Storage
	sch          *scheduler.Scheduler
	pixiv        *pixiv.Client
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	pendingLogin *pixiv.LoginFlow
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

	sch := scheduler.New(cfg, st, client, setter)
	if err = sch.RebuildQueue(); err != nil {
		logger.WithComponent(componentName).Warn("Failed to rebuild queue after cleanup", "error", err)
	}

	return &Controller{
		cfg:    cfg,
		st:     st,
		sch:    sch,
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

	go c.generateMissingThumbnails(images)

	return nil
}

func (c *Controller) generateMissingThumbnails(images map[string]*storage.ImageMeta) {
	log := logger.WithComponent(componentName)
	for id, meta := range images {
		if meta.Path == "" {
			continue
		}

		if err := c.st.GenerateThumbnail(meta.Path, id); err != nil {
			log.Warn("Failed to generate thumbnail", "id", id, "error", err)
		}
	}
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

// PauseRotation disables wallpaper rotation in the scheduler.
func (c *Controller) PauseRotation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cfg.Wallpaper.RotationEnabled {
		return
	}
	c.cfg.Wallpaper.RotationEnabled = false
	c.sch.ApplyConfig(c.cfg)
}

// ResumeRotation enables wallpaper rotation in the scheduler.
func (c *Controller) ResumeRotation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Wallpaper.RotationEnabled {
		return
	}
	c.cfg.Wallpaper.RotationEnabled = true
	c.sch.ApplyConfig(c.cfg)
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

// ExcludeCurrentWallpaper blacklists the current wallpaper, deletes the file, and switches to another one.
func (c *Controller) ExcludeCurrentWallpaper() error {
	log := logger.WithComponent("app")

	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}

	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}

	metadata, err := c.st.LoadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	if meta, ok := metadata[currentID]; ok {
		if rErr := os.Remove(meta.Path); rErr != nil && !os.IsNotExist(rErr) {
			log.Warn("Failed to delete excluded wallpaper file", "id", currentID, "path", meta.Path, "error", rErr)
		} else {
			log.Debug("Deleted excluded wallpaper file", "id", currentID, "path", meta.Path)
		}

		delete(metadata, currentID)

		if sErr := c.st.SaveMetadata(metadata); sErr != nil {
			log.Warn("Failed to save metadata after exclusion", "error", sErr)
		}
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

// BeginLogin starts the Pixiv account login flow and returns the authorization URL.
func (c *Controller) BeginLogin() (string, error) {
	if c.pixiv == nil {
		return "", fmt.Errorf("pixiv account actions are unavailable in dry-run mode")
	}

	flow, err := c.pixiv.BeginLogin()
	if err != nil {
		return "", err
	}

	c.pendingLogin = flow
	return flow.URL, nil
}

// FinishLogin completes the login flow with the callback code from Pixiv.
func (c *Controller) FinishLogin(callbackCode string) error {
	if c.pendingLogin == nil {
		return fmt.Errorf("no login flow in progress")
	}

	_, err := c.pixiv.FinishLogin(c.ctx, c.pendingLogin.CodeVerifier, callbackCode)
	c.pendingLogin = nil
	if err != nil {
		return err
	}

	return nil
}

// LoginToPixiv starts the Pixiv account login flow using desktop dialogs (kdialog/zenity).
func (c *Controller) LoginToPixiv() error {
	url, err := c.BeginLogin()
	if err != nil {
		return err
	}

	if err = openExternal(url); err != nil {
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

	return c.FinishLogin(code)
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

	if c.IsArtworkBookmarked() {
		logger.WithComponent(componentName).Debug("artwork already bookmarked, skipping", "id", currentID)
		return nil
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
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
	if c.sch != nil {
		c.sch.ApplyConfig(cfg)
	}
}

// Config returns the current application configuration.
func (c *Controller) Config() *config.Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// FetchNow triggers an immediate wallpaper fetch in the background.
func (c *Controller) FetchNow() error {
	if c.pixiv == nil {
		return fmt.Errorf("pixiv client not available")
	}
	c.sch.FetchNow(c.ctx, componentName)
	return nil
}

// SchedulerRunning reports whether the scheduler is active.
func (c *Controller) SchedulerRunning() bool {
	if c.sch == nil {
		return false
	}
	return c.sch.IsRunning()
}

// CurrentWallpaper returns metadata about the current wallpaper.
// Returns nil metdata (no error) when no wallpaper has been set yet.
func (c *Controller) CurrentWallpaper() (*storage.ImageMeta, error) {
	currentID, cwErr := c.st.GetCurrentWallpaper()
	if cwErr != nil {
		return nil, cwErr
	}
	if currentID == "" {
		return nil, nil //nolint:nilnil // no wallpaper set yet, not an error
	}

	metaMap, ldErr := c.st.LoadMetadata()
	if ldErr != nil {
		return nil, ldErr
	}

	meta, ok := metaMap[currentID]
	if !ok {
		return nil, nil //nolint:nilnil // wallpaper ID not found in metadata
	}

	return meta, nil
}

// CachedCount returns the number of images in the metadata store.
func (c *Controller) CachedCount() int {
	meta, err := c.st.LoadMetadata()
	if err != nil {
		return 0
	}
	return len(meta)
}

// LastRotation returns the timestamp of the last wallpaper rotation.
func (c *Controller) LastRotation() time.Time {
	history, err := c.st.LoadHistory()
	if err != nil {
		return time.Time{}
	}
	return history.UpdatedAt
}

// SyncBookmarks triggers an immediate bookmark sync.
func (c *Controller) SyncBookmarks() error {
	if !c.PixivLoggedIn() {
		return fmt.Errorf("pixiv login required")
	}

	if !c.cfg.Bookmarks.Enabled {
		return fmt.Errorf("bookmark sync is disabled in config")
	}

	return c.sch.SyncBookmarksNowSync(c.ctx, componentName)
}

// ShowSettingsWindow opens the settings window without blocking the tray.
func (c *Controller) ShowSettingsWindow() error {
	logger.WithComponent(componentName).Debug("Opening settings window")
	gui.ShowSettings(c, gui.HomePage)
	return nil
}

// ShowAccountSettings opens the settings window at the Account tab.
func (c *Controller) ShowAccountSettings() error {
	logger.WithComponent(componentName).Debug("Opening settings window at account tab")
	gui.ShowSettings(c, gui.AccountPage)
	return nil
}

func (c *Controller) ThumbnailPath(id string) string {
	return c.st.ThumbnailPath(id)
}

func (c *Controller) ServiceEnabled() (bool, error) {
	return platform.IsServiceEnabled("kpixiv.service")
}

func (c *Controller) EnableService() error {
	return platform.EnableService("kpixiv.service")
}

func (c *Controller) DisableService() error {
	return platform.DisableService("kpixiv.service")
}

func openExternal(target string) error {
	return exec.Command("xdg-open", target).Start() //nolint:gosec // target is generated internally or a local path.
}
