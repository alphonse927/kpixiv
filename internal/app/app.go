package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/auth"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/gui"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/notify"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/platform"
	"github.com/alphonse927/kpixiv/internal/scheduler"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

const componentName = "app"

// cacheStatsTTL bounds how often cache statistics are recomputed. The GUI
// refreshes status every few seconds, so results are reused within the TTL
// to avoid repeated filesystem stats.
const cacheStatsTTL = 30 * time.Second

type Controller struct {
	cfg         *config.Config
	st          *storage.Storage
	sch         *scheduler.Scheduler
	setter      wallpaper.Setter
	pixiv       *pixiv.Client
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	rebuildMenu func()
	rebuildGUI  func()

	// multiMonitorEnabled tracks the last known multi-monitor state. The GUI
	// mutates the shared config pointer in place before ApplyConfig, so the
	// toggle cannot be detected by comparing the old and new configs.
	multiMonitorEnabled bool

	statsMu      sync.Mutex
	cachedStats  storage.CacheStats
	statsUpdated time.Time
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

	notify.SetEnabled(cfg.Notifications.Enabled)

	if result, cleanupErr := st.CleanupImagesOlderThanDays(cleanupDays); cleanupErr != nil {
		logger.WithComponent(componentName).Warn("Failed to cleanup old images", "error", cleanupErr)
	} else if result.Removed > 0 {
		logger.WithComponent(componentName).Info(
			"Removed unused wallpapers",
			"removed", result.Removed,
			"freedBytes", result.FreedBytes,
			"days", cleanupDays,
		)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sch := scheduler.New(cfg, st, client, setter)
	if err = sch.RebuildQueue(); err != nil {
		logger.WithComponent(componentName).Warn("Failed to rebuild queue after cleanup", "error", err)
	}

	return &Controller{
		cfg:                 cfg,
		st:                  st,
		sch:                 sch,
		setter:              setter,
		pixiv:               client,
		ctx:                 ctx,
		cancel:              cancel,
		multiMonitorEnabled: cfg.Wallpaper.MultiMonitorEnabled,
	}, nil
}

// Monitors returns the screens currently reported by the desktop provider.
func (c *Controller) Monitors() ([]wallpaper.Screen, error) {
	setter, ok := c.setter.(wallpaper.MonitorSetter)
	if !ok {
		return nil, fmt.Errorf("wallpaper provider does not support monitor discovery")
	}
	return setter.Screens()
}

// MonitorWallpapers returns the currently remembered wallpaper for each
// active screen.
func (c *Controller) MonitorWallpapers() (map[string]*storage.ImageMeta, error) {
	screens, err := c.Monitors()
	if err != nil {
		return nil, err
	}

	monitors, err := c.st.LoadMonitorHistory()
	if err != nil {
		return nil, err
	}

	metadata, err := c.st.LoadMetadata()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*storage.ImageMeta)
	globalID, _ := c.st.GetCurrentWallpaper() //nolint:errcheck // monitor status has a best-effort fallback
	for _, screen := range screens {
		id := monitors[screen.ID]
		if id == "" {
			id = globalID
		}

		if id == "" {
			continue
		}

		if meta, ok := metadata[id]; ok {
			result[screen.ID] = meta
			continue
		}

		// Metadata missing (e.g., after cleanup or queue refill). Fall back to
		// finding the file on disk so the GUI can still display a wallpaper is set.
		path, exists := c.st.GetImagePath(id)
		if !exists {
			continue
		}

		result[screen.ID] = &storage.ImageMeta{ID: id, Path: path}
	}

	return result, nil
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
	var setErr error
	if c.cfg.Wallpaper.MultiMonitorEnabled {
		setErr = c.sch.SetNextWallpapers(trayComponentName)
	} else {
		sq := storage.NewQueue(c.st.StateDir())
		if err := sq.Load(); err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}
		setErr = c.sch.SetNextWallpaper(sq, trayComponentName)
	}

	if err := setErr; err != nil {
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

// OpenWallpaperFile opens the wallpaper file for the given artwork ID in the default app.
func (c *Controller) OpenWallpaperFile(artworkID string) error {
	if artworkID == "" {
		return fmt.Errorf("no artwork ID provided")
	}
	path, ok := c.st.GetImagePath(artworkID)
	if !ok {
		return fmt.Errorf("artwork not found on disk")
	}
	//nolint:gosec // xdg-open is intentionally used with a local filesystem path.
	return exec.Command("xdg-open", path).Start()
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
	return c.OpenWallpaperFile(currentID)
}

// OpenWallpaperInPixiv opens the Pixiv page for the given artwork ID.
func (c *Controller) OpenWallpaperInPixiv(artworkID string) error {
	if artworkID == "" {
		return fmt.Errorf("no artwork ID provided")
	}
	url := fmt.Sprintf("https://www.pixiv.net/artworks/%s", artworkID)
	logger.WithComponent(componentName).Debug("opening artwork in Pixiv", "id", artworkID, "url", url)
	//nolint:gosec // url is constructed from a trusted internal ID.
	return exec.Command("xdg-open", url).Start()
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
	return c.OpenWallpaperInPixiv(currentID)
}

// ExcludeWallpaper blacklists an artwork, deletes its file, and switches to the next one.
func (c *Controller) ExcludeWallpaper(artworkID string) error {
	log := logger.WithComponent("app")

	if artworkID == "" {
		return fmt.Errorf("no artwork ID provided")
	}

	metadata, err := c.st.LoadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	if meta, ok := metadata[artworkID]; ok {
		if rErr := os.Remove(meta.Path); rErr != nil && !os.IsNotExist(rErr) {
			log.Warn("Failed to delete excluded wallpaper file", "id", artworkID, "path", meta.Path, "error", rErr)
		} else {
			log.Debug("Deleted excluded wallpaper file", "id", artworkID, "path", meta.Path)
		}
		delete(metadata, artworkID)
		if sErr := c.st.SaveMetadata(metadata); sErr != nil {
			log.Warn("Failed to save metadata after exclusion", "error", sErr)
		}
	}

	if err = c.st.ExcludeWallpaper(artworkID); err != nil {
		return fmt.Errorf("failed to exclude artwork: %w", err)
	}

	q := storage.NewQueue(c.st.StateDir())
	if err = q.Load(); err != nil {
		return fmt.Errorf("excluded artwork, but failed to load queue: %w", err)
	}
	if err = q.Remove(artworkID); err != nil {
		return fmt.Errorf("excluded artwork, but failed to update queue: %w", err)
	}

	if c.cfg.Wallpaper.MultiMonitorEnabled {
		if err = c.sch.SetNextWallpapers(trayComponentName); err != nil {
			return err
		}
	} else {
		if err = c.sch.SetNextWallpaper(q, trayComponentName); err != nil {
			return err
		}
	}

	c.sch.ResetRotationTimer()
	notify.SendDefault("KPixiv", fmt.Sprintf("The image %s has been excluded from the normal rotation.", c.artworkTitle(artworkID)))
	return nil
}

// ExcludeCurrentWallpaper blacklists the current wallpaper, deletes the file, and switches to another one.
func (c *Controller) ExcludeCurrentWallpaper() error {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}
	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}
	return c.ExcludeWallpaper(currentID)
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

// SetGUIRefresher registers a callback invoked when auth state changes.
func (c *Controller) SetGUIRefresher(fn func()) {
	c.rebuildGUI = fn
}

// refreshUI signals both tray and GUI to rebuild after auth state changes.
func (c *Controller) refreshUI() {
	if c.rebuildMenu != nil {
		c.rebuildMenu()
	}
	if c.rebuildGUI != nil {
		c.rebuildGUI()
	}
}

// AutoLogin performs the complete OAuth login flow automatically.
func (c *Controller) AutoLogin(ctx context.Context, cfg auth.LoginConfig) error {
	if c.pixiv == nil {
		return fmt.Errorf("pixiv account actions are unavailable in dry-run mode")
	}

	_, err := auth.Login(ctx, cfg, &pixiv.AuthProvider{Client: c.pixiv})
	if err == nil {
		c.refreshUI()
	}

	return err
}

// LogoutFromPixiv removes the stored Pixiv session.
func (c *Controller) LogoutFromPixiv() error {
	if c.pixiv == nil {
		return nil
	}

	if err := c.pixiv.Logout(); err != nil {
		return err
	}

	c.refreshUI()

	return nil
}

// CopyWallpaperToFavorites copies an artwork to the configured download directory.
func (c *Controller) CopyWallpaperToFavorites(artworkID string) error {
	if artworkID == "" {
		return fmt.Errorf("no artwork ID provided")
	}
	logger.WithComponent(componentName).Debug("copying artwork to favorites", "id", artworkID)
	destPath, err := c.st.CopyImageToDownloadDir(artworkID)
	if err != nil {
		return err
	}
	logger.WithComponent(componentName).Debug("artwork copied", "id", artworkID, "dest", destPath)
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
	return c.CopyWallpaperToFavorites(currentID)
}

// BookmarkWallpaper bookmarks a specific artwork in Pixiv.
func (c *Controller) BookmarkWallpaper(artworkID string) error {
	if !c.PixivLoggedIn() {
		return fmt.Errorf("pixiv login required")
	}
	if artworkID == "" {
		return fmt.Errorf("no artwork ID provided")
	}

	ok, err := c.st.IsArtworkBookmarked(artworkID)
	if err == nil && ok {
		logger.WithComponent(componentName).Debug("artwork already bookmarked, skipping", "id", artworkID)
		return nil
	}

	logger.WithComponent(componentName).Debug("bookmarking artwork on Pixiv", "id", artworkID)
	if err = c.pixiv.BookmarkIllust(c.ctx, artworkID); err != nil {
		return err
	}

	logger.WithComponent(componentName).Debug("artwork bookmarked on Pixiv, saving locally", "id", artworkID)
	if err = c.st.AddBookmark(artworkID); err != nil {
		logger.WithComponent(componentName).Warn("bookmarked on Pixiv but failed to save locally", "id", artworkID, "error", err)
	}

	notify.SendDefault("KPixiv", fmt.Sprintf("The image %s has been bookmarked on Pixiv.", c.artworkTitle(artworkID)))
	return nil
}

// artworkTitle returns the stored title for an artwork, falling back to its ID.
func (c *Controller) artworkTitle(id string) string {
	metadata, err := c.st.LoadMetadata()
	if err != nil {
		return id
	}

	if meta, ok := metadata[id]; ok && meta.Title != "" {
		return meta.Title
	}

	return id
}

// BookmarkCurrentArtwork bookmarks the current artwork in Pixiv.
func (c *Controller) BookmarkCurrentArtwork() error {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return err
	}
	if currentID == "" {
		return fmt.Errorf("no current artwork")
	}
	return c.BookmarkWallpaper(currentID)
}

// IsWallpaperBookmarked returns whether the given artwork has been locally bookmarked.
func (c *Controller) IsWallpaperBookmarked(artworkID string) bool {
	if artworkID == "" {
		return false
	}

	ok, err := c.st.IsArtworkBookmarked(artworkID)
	if err != nil {
		return false
	}

	return ok
}

// CurrentWallpaperID returns the ID of the currently active wallpaper, or ""
// when none is set.
func (c *Controller) CurrentWallpaperID() string {
	currentID, err := c.st.GetCurrentWallpaper()
	if err != nil {
		return ""
	}
	return currentID
}

// Shutdown cancels running work and stops the scheduler.
func (c *Controller) Shutdown() {
	c.cancel()
	c.sch.Stop(componentName)
}

// ApplyConfig applies a new config to the scheduler without restarting it.
func (c *Controller) ApplyConfig(cfg *config.Config) {
	c.mu.Lock()
	multiToggled := c.multiMonitorEnabled != cfg.Wallpaper.MultiMonitorEnabled
	c.multiMonitorEnabled = cfg.Wallpaper.MultiMonitorEnabled
	c.cfg = cfg
	c.mu.Unlock()

	notify.SetEnabled(cfg.Notifications.Enabled)
	if c.sch != nil {
		c.sch.ApplyConfig(cfg)
		if multiToggled {
			// Restore the last wallpaper used on each screen (or globally)
			// when multi-monitor mode is switched on or off.
			if err := c.sch.ApplyCurrentOrNext(); err != nil {
				logger.WithComponent(componentName).Warn("Failed to change wallpaper after multi-monitor toggle", "error", err)
			}
		}
	}

	if c.rebuildMenu != nil {
		c.rebuildMenu()
	}
}

// SetTrayRebuilder registers a function that rebuilds the tray menu on config changes.
func (c *Controller) SetTrayRebuilder(fn func()) {
	c.rebuildMenu = fn
}

// MultiMonitorEnabled returns whether multi-monitor wallpaper mode is active.
func (c *Controller) MultiMonitorEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.Wallpaper.MultiMonitorEnabled
}

// NextWallpaperForMonitor rotates the wallpaper on a single monitor.
func (c *Controller) NextWallpaperForMonitor(monitorID string) error {
	q := storage.NewQueue(c.st.StateDir())
	if err := q.Load(); err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}

	screen := c.sch.ResolveScreen(monitorID)
	if screen == nil {
		return fmt.Errorf("monitor %q not found", monitorID)
	}

	if err := c.sch.SetNextWallpaperForScreen(q, screen.ID, screen.Index, trayComponentName); err != nil {
		return err
	}

	c.sch.ResetRotationTimer()
	return nil
}

// NextWallpaperForAllMonitors rotates wallpapers on all monitors.
func (c *Controller) NextWallpaperForAllMonitors() error {
	if err := c.sch.SetNextWallpapers(trayComponentName); err != nil {
		return err
	}
	c.sch.ResetRotationTimer()
	return nil
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
	if ok {
		return meta, nil
	}

	// Wallpaper ID exists in history, but metadata is missing (e.g., after
	// cleanup or queue refill from filesystem). Fall back to finding the
	// file on disk so the GUI can still display that a wallpaper is set.
	path, exists := c.st.GetImagePath(currentID)
	if !exists {
		return nil, nil //nolint:nilnil // wallpaper ID not found in metadata
	}

	return &storage.ImageMeta{
		ID:   currentID,
		Path: path,
	}, nil
}

// CachedCount returns the number of images in the metadata store.
func (c *Controller) CachedCount() int {
	stats, err := c.CacheStats()
	if err != nil {
		return 0
	}
	return stats.Count
}

// CacheStats returns statistics about the downloaded wallpaper cache.
// Results are reused for cacheStatsTTL to keep status refreshes cheap.
func (c *Controller) CacheStats() (storage.CacheStats, error) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	if !c.statsUpdated.IsZero() && time.Since(c.statsUpdated) < cacheStatsTTL {
		return c.cachedStats, nil
	}

	stats, err := c.st.CacheStats()
	if err != nil {
		return storage.CacheStats{}, err
	}
	c.cachedStats = stats
	c.statsUpdated = time.Now()
	return stats, nil
}

// HistoryCount returns the number of remembered wallpaper history entries.
func (c *Controller) HistoryCount() int {
	history, err := c.st.LoadHistory()
	if err != nil {
		return 0
	}
	count := len(history.Images)
	if history.Current != "" {
		count++
	}
	return count
}

// NextWallpaperID returns the ID of the next wallpaper in the rotation queue.
func (c *Controller) NextWallpaperID() string {
	q := storage.NewQueue(c.st.StateDir())
	if err := q.Load(); err != nil {
		return ""
	}
	id, ok := q.Peek()
	if !ok {
		return ""
	}
	return id
}

// MonitorNextWallpapers returns the next queued wallpaper ID for each active
// screen when multi-monitor rotation is enabled.
func (c *Controller) MonitorNextWallpapers() map[string]string {
	result := make(map[string]string)
	screens, err := c.Monitors()
	if err != nil {
		return result
	}

	for _, screen := range screens {
		mq := storage.NewMonitorQueue(c.st.StateDir(), screen.ID)
		if err = mq.Load(); err != nil {
			continue
		}

		if id, ok := mq.Peek(); ok {
			result[screen.ID] = id
		}
	}

	return result
}

// WallpaperMeta returns metadata for a specific artwork ID, or nil when the
// artwork is not in the metadata store.
func (c *Controller) WallpaperMeta(id string) (*storage.ImageMeta, error) {
	if id == "" {
		return nil, nil //nolint:nilnil // empty id is not an error
	}
	meta, err := c.st.LoadMetadata()
	if err != nil {
		return nil, err
	}
	return meta[id], nil
}

// LastRotation returns the timestamp of the last wallpaper rotation.
func (c *Controller) LastRotation() time.Time {
	history, err := c.st.LoadHistory()
	if err != nil {
		return time.Time{}
	}
	return history.UpdatedAt
}

// LastFetch returns the timestamp of the last ranking fetch.
func (c *Controller) LastFetch() time.Time {
	activity, err := c.st.LoadActivity()
	if err != nil {
		return time.Time{}
	}
	return activity.LastFetchAt
}

// LastBookmarkSync returns the timestamp of the last bookmark sync.
func (c *Controller) LastBookmarkSync() time.Time {
	activity, err := c.st.LoadActivity()
	if err != nil {
		return time.Time{}
	}
	return activity.LastBookmarkSyncAt
}

// FetchInProgress reports whether a ranking fetch is actively running right now.
func (c *Controller) FetchInProgress() bool {
	return c.sch.FetchInProgress()
}

// LastFetchAttempt returns when the most recent fetch attempt started,
// regardless of whether it succeeded.
func (c *Controller) LastFetchAttempt() time.Time {
	return c.sch.LastFetchAttempt()
}

// LastFetchError returns the error from the most recent fetch attempt, or
// nil if it succeeded (or none has run yet).
func (c *Controller) LastFetchError() error {
	return c.sch.LastFetchError()
}

// BookmarkSyncInProgress reports whether a bookmark sync is actively running right now.
func (c *Controller) BookmarkSyncInProgress() bool {
	return c.sch.BookmarkSyncInProgress()
}

// LastBookmarkSyncAttempt returns when the most recent bookmark sync attempt
// started, regardless of whether it succeeded.
func (c *Controller) LastBookmarkSyncAttempt() time.Time {
	return c.sch.LastBookmarkSyncAttempt()
}

// LastBookmarkSyncError returns the error from the most recent bookmark sync
// attempt, or nil if it succeeded (or none has run yet).
func (c *Controller) LastBookmarkSyncError() error {
	return c.sch.LastBookmarkSyncError()
}

// ConfigPath returns the path of the active configuration file.
func (c *Controller) ConfigPath() string {
	return c.cfg.ConfigPath
}

// DataDir returns the application data directory (wallpaper cache).
func (c *Controller) DataDir() string {
	return c.st.DataDir()
}

// StateDir returns the application state directory (queue, history, session).
func (c *Controller) StateDir() string {
	return c.st.StateDir()
}

// DownloadDir returns the user-facing download directory.
func (c *Controller) DownloadDir() string {
	return c.st.DownloadDir()
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

// EnsureThumbnail returns the thumbnail path for an artwork, generating it on
// demand when the cached copy is missing.
func (c *Controller) EnsureThumbnail(id string) string {
	path := c.st.ThumbnailPath(id)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	meta, err := c.WallpaperMeta(id)
	if err != nil || meta == nil || meta.Path == "" {
		return path
	}

	if err = c.st.GenerateThumbnail(meta.Path, id); err != nil {
		logger.WithComponent(componentName).Warn("Failed to generate thumbnail", "id", id, "error", err)
	}

	return path
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
