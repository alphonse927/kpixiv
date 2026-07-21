package tray

import (
	"context"
	"log/slog"
	"time"

	"fyne.io/systray"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

type Controller interface {
	Start() error
	NextWallpaper() error
	PauseRotation()
	ResumeRotation()
	PixivLoggedIn() bool
	PixivUserName() string
	LoginToPixiv() error
	LogoutFromPixiv() error
	CopyCurrentArtwork() error
	BookmarkCurrentArtwork() error
	IsArtworkBookmarked() bool
	OpenCurrentArtwork() error
	OpenCurrentArtworkInPixiv() error
	ExcludeCurrentWallpaper() error

	MultiMonitorEnabled() bool
	Monitors() ([]wallpaper.Screen, error)
	MonitorWallpapers() (map[string]*storage.ImageMeta, error)
	NextWallpaperForMonitor(monitorID string) error
	NextWallpaperForAllMonitors() error
	BookmarkWallpaper(artworkID string) error
	ExcludeWallpaper(artworkID string) error
	OpenWallpaperFile(artworkID string) error
	OpenWallpaperInPixiv(artworkID string) error
	CopyWallpaperToFavorites(artworkID string) error

	ShowSettingsWindow() error
	ShowAccountSettings() error
	Shutdown()
}

type trayMenu struct {
	ctrl Controller
	log  *slog.Logger

	next            *systray.MenuItem
	rotate          *systray.MenuItem
	login           *systray.MenuItem
	bookmarkCurrent *systray.MenuItem
	logout          *systray.MenuItem
	favoriteCurrent *systray.MenuItem
	openCurrent     *systray.MenuItem
	openPixiv       *systray.MenuItem
	excludeCurrent  *systray.MenuItem
	settings        *systray.MenuItem
	quit            *systray.MenuItem

	rebuildCh chan struct{}
}

// Run starts the tray event loop and wires it to the application controller.
func Run(appCtx context.Context, controller Controller) {
	systray.Run(func() { onReady(appCtx, controller) }, func() {
		controller.Shutdown()
	})
}

func onReady(appCtx context.Context, controller Controller) {
	tm := &trayMenu{
		ctrl:      controller,
		log:       logger.WithComponent("tray"),
		rebuildCh: make(chan struct{}, 1),
	}

	systray.SetTitle("kPixiv")
	systray.SetTooltip("kPixiv Wallpaper Manager")

	if icon := loadIconPNG(); len(icon) > 0 {
		systray.SetIcon(icon)
	}

	if err := controller.Start(); err != nil {
		tm.log.Error("Failed to start application controller", "error", err)
	}

	tm.next = systray.AddMenuItem("Next Wallpaper", "Immediately switch wallpaper")
	tm.rotate = systray.AddMenuItemCheckbox("Rotate Wallpaper", "Enable or pause wallpaper rotation", true)

	systray.AddSeparator()

	tm.login = systray.AddMenuItem("Login to Pixiv", "Connect your Pixiv account")
	tm.bookmarkCurrent = systray.AddMenuItem("Bookmark Current Artwork", "Bookmark the current artwork in Pixiv")
	tm.logout = systray.AddMenuItem("Logout from Pixiv", "Forget the saved Pixiv session")

	systray.AddSeparator()

	tm.favoriteCurrent = systray.AddMenuItem("Copy to Favorites", "Copy the current wallpaper to the favorite directory")
	tm.openCurrent = systray.AddMenuItem("Open Current Artwork", "Open currently active image")
	tm.openPixiv = systray.AddMenuItem("Open Artwork in Pixiv", "Open the current artwork's Pixiv page in your browser")
	tm.excludeCurrent = systray.AddMenuItem("Exclude Current Wallpaper", "Blacklist the current wallpaper and switch away")

	systray.AddSeparator()
	tm.settings = systray.AddMenuItem("Settings", "Open settings window")
	tm.quit = systray.AddMenuItem("Quit", "Quit kPixiv")

	// Register as rebuild target (socket-ready foundation)
	if r, ok := controller.(interface{ SetTrayRebuilder(func()) }); ok {
		r.SetTrayRebuilder(tm.RebuildMenu)
	}

	tm.updateAuthItems()
	go tm.eventLoop(appCtx)
}

//nolint:cyclop
func (tm *trayMenu) eventLoop(appCtx context.Context) {
	bookmarkTicker := time.NewTicker(3 * time.Second)
	defer bookmarkTicker.Stop()

	for {
		select {
		case <-tm.next.ClickedCh:
			if err := tm.ctrl.NextWallpaper(); err != nil {
				tm.log.Warn("Failed to set next wallpaper", "error", err)
			}
			tm.updateBookmarkItem()

		case <-tm.rotate.ClickedCh:
			if tm.rotate.Checked() {
				tm.rotate.Uncheck()
				tm.ctrl.PauseRotation()
			} else {
				tm.rotate.Check()
				tm.ctrl.ResumeRotation()
			}

		case <-tm.login.ClickedCh:
			if err := tm.ctrl.ShowAccountSettings(); err != nil {
				tm.log.Warn("Failed to open account settings", "error", err)
			}

		case <-tm.logout.ClickedCh:
			if err := tm.ctrl.LogoutFromPixiv(); err != nil {
				tm.log.Warn("Failed to log out from Pixiv", "error", err)
			}
			tm.updateAuthItems()

		case <-tm.favoriteCurrent.ClickedCh:
			if err := tm.ctrl.CopyCurrentArtwork(); err != nil {
				tm.log.Warn("Failed to copy current artwork", "error", err)
			}

		case <-tm.bookmarkCurrent.ClickedCh:
			if err := tm.ctrl.BookmarkCurrentArtwork(); err != nil {
				tm.log.Warn("Failed to bookmark current artwork", "error", err)
			}
			tm.updateBookmarkItem()

		case <-tm.openCurrent.ClickedCh:
			if err := tm.ctrl.OpenCurrentArtwork(); err != nil {
				tm.log.Warn("Failed to open current artwork", "error", err)
			}

		case <-tm.openPixiv.ClickedCh:
			if err := tm.ctrl.OpenCurrentArtworkInPixiv(); err != nil {
				tm.log.Warn("Failed to open current artwork in Pixiv", "error", err)
			}

		case <-tm.excludeCurrent.ClickedCh:
			if err := tm.ctrl.ExcludeCurrentWallpaper(); err != nil {
				tm.log.Warn("Failed to exclude current artwork", "error", err)
			}
			tm.updateBookmarkItem()

		case <-tm.settings.ClickedCh:
			if err := tm.ctrl.ShowSettingsWindow(); err != nil {
				tm.log.Warn("Failed to open settings", "error", err)
			}

		case <-tm.quit.ClickedCh:
			tm.ctrl.Shutdown()
			systray.Quit()
			return

		case <-bookmarkTicker.C:
			tm.updateBookmarkItem()

		case <-tm.rebuildCh:
			tm.updateBookmarkItem()
			tm.updateAuthItems()

		case <-appCtx.Done():
			systray.Quit()
			return
		}
	}
}

// RebuildMenu is called by the controller when config changes (e.g. multi-monitor toggle).
// Currently refreshes auth/bookmark state; in the socket architecture it will rebuild the
// full menu from a state message sent by the daemon.
func (tm *trayMenu) RebuildMenu() {
	select {
	case tm.rebuildCh <- struct{}{}:
	default:
	}
}

func (tm *trayMenu) updateAuthItems() {
	if tm.ctrl.PixivLoggedIn() {
		userName := tm.ctrl.PixivUserName()
		if userName != "" {
			tm.login.SetTitle("Pixiv: " + userName)
		} else {
			tm.login.SetTitle("Pixiv Connected")
		}
		tm.login.Disable()
		tm.logout.Show()
		tm.logout.Enable()
		tm.bookmarkCurrent.Enable()
		tm.updateBookmarkItem()
		return
	}

	tm.login.SetTitle("Login to Pixiv")
	tm.login.Enable()
	tm.logout.Hide()
	tm.logout.Disable()
	tm.bookmarkCurrent.Disable()
	tm.bookmarkCurrent.SetTitle("Bookmark Current Artwork")
}

func (tm *trayMenu) updateBookmarkItem() {
	if tm.ctrl.IsArtworkBookmarked() {
		tm.bookmarkCurrent.SetTitle("Bookmarked")
		tm.bookmarkCurrent.Disable()
	} else {
		tm.bookmarkCurrent.SetTitle("Bookmark Current Artwork")
		tm.bookmarkCurrent.Enable()
	}
}
