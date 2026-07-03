package tray

import (
	"context"
	"time"

	"fyne.io/systray"
	"github.com/alphonse927/kpixiv/internal/logger"
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
	ShowSettingsWindow() error
	Shutdown()
}

// Run starts the tray event loop and wires it to the application controller.
func Run(appCtx context.Context, controller Controller) {
	systray.Run(func() { onReady(appCtx, controller) }, func() {
		controller.Shutdown()
	})
}

//nolint:cyclop,funlen // straightforward select handler with many menu cases
func onReady(appCtx context.Context, controller Controller) {
	log := logger.WithComponent("tray")
	systray.SetTitle("kPixiv")
	systray.SetTooltip("kPixiv Wallpaper Manager")

	if icon := loadIconPNG(); len(icon) > 0 {
		systray.SetIcon(icon)
	}

	if err := controller.Start(); err != nil {
		log.Error("Failed to start application controller", "error", err)
	}

	next := systray.AddMenuItem("Next Wallpaper", "Immediately switch wallpaper")
	rotate := systray.AddMenuItemCheckbox("Rotate Wallpaper", "Enable or pause wallpaper rotation", true)

	systray.AddSeparator()

	login := systray.AddMenuItem("Login to Pixiv", "Connect your Pixiv account")
	bookmarkCurrent := systray.AddMenuItem("Bookmark Current Artwork", "Bookmark the current artwork in Pixiv")
	logout := systray.AddMenuItem("Logout from Pixiv", "Forget the saved Pixiv session")

	systray.AddSeparator()

	favoriteCurrent := systray.AddMenuItem("Copy to Favorites", "Copy the current wallpaper to the favorite directory")
	openCurrent := systray.AddMenuItem("Open Current Artwork", "Open currently active image")
	openPixiv := systray.AddMenuItem("Open Artwork in Pixiv", "Open the current artwork's Pixiv page in your browser")
	excludeCurrent := systray.AddMenuItem("Exclude Current Wallpaper", "Blacklist the current wallpaper and switch away")

	systray.AddSeparator()
	settings := systray.AddMenuItem("Settings", "Open settings window")
	quit := systray.AddMenuItem("Quit", "Quit kPixiv")

	updateBookmarkItem := func() {
		if controller.IsArtworkBookmarked() {
			bookmarkCurrent.SetTitle("Bookmarked")
			bookmarkCurrent.Disable()
		} else {
			bookmarkCurrent.SetTitle("Bookmark Current Artwork")
			bookmarkCurrent.Enable()
		}
	}
	updateAuthItems := func() {
		if controller.PixivLoggedIn() {
			userName := controller.PixivUserName()
			if userName != "" {
				login.SetTitle("Pixiv: " + userName)
			} else {
				login.SetTitle("Pixiv Connected")
			}
			login.Disable()
			logout.Show()
			logout.Enable()
			bookmarkCurrent.Enable()
			updateBookmarkItem()
			return
		}

		login.SetTitle("Login to Pixiv")
		login.Enable()
		logout.Hide()
		logout.Disable()
		bookmarkCurrent.Disable()
		bookmarkCurrent.SetTitle("Bookmark Current Artwork")
	}
	updateAuthItems()

	go func() {
		bookmarkTicker := time.NewTicker(3 * time.Second)
		defer bookmarkTicker.Stop()
		for {
			select {
			case <-next.ClickedCh:
				if err := controller.NextWallpaper(); err != nil {
					log.Warn("Failed to set next wallpaper", "error", err)
				}
				updateBookmarkItem()
			case <-rotate.ClickedCh:
				if rotate.Checked() {
					rotate.Uncheck()
					controller.PauseRotation()
					log.Debug("Rotation paused")
				} else {
					rotate.Check()
					controller.ResumeRotation()
					log.Debug("Rotation resumed")
				}
			case <-login.ClickedCh:
				if err := controller.LoginToPixiv(); err != nil {
					log.Warn("Failed to log in to Pixiv", "error", err)
				}
				updateAuthItems()
			case <-logout.ClickedCh:
				if err := controller.LogoutFromPixiv(); err != nil {
					log.Warn("Failed to log out from Pixiv", "error", err)
				}
				updateAuthItems()
			case <-favoriteCurrent.ClickedCh:
				if err := controller.CopyCurrentArtwork(); err != nil {
					log.Warn("Failed to copy current artwork", "error", err)
				}
			case <-bookmarkCurrent.ClickedCh:
				if err := controller.BookmarkCurrentArtwork(); err != nil {
					log.Warn("Failed to bookmark current artwork", "error", err)
				}
				updateBookmarkItem()
			case <-openCurrent.ClickedCh:
				if err := controller.OpenCurrentArtwork(); err != nil {
					log.Warn("Failed to open current artwork", "error", err)
				}
			case <-openPixiv.ClickedCh:
				if err := controller.OpenCurrentArtworkInPixiv(); err != nil {
					log.Warn("Failed to open current artwork in Pixiv", "error", err)
				}
			case <-excludeCurrent.ClickedCh:
				if err := controller.ExcludeCurrentWallpaper(); err != nil {
					log.Warn("Failed to exclude current artwork", "error", err)
				}
				updateBookmarkItem()
			case <-settings.ClickedCh:
				if err := controller.ShowSettingsWindow(); err != nil {
					log.Warn("Failed to open settings", "error", err)
				}
			case <-quit.ClickedCh:
				controller.Shutdown()
				systray.Quit()
				return
			case <-bookmarkTicker.C:
				updateBookmarkItem()
			case <-appCtx.Done():
				systray.Quit()
				return
			}
		}
	}()
}
