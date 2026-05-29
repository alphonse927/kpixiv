package tray

import (
	"context"

	"fyne.io/systray"
	"github.com/alphonse927/kpixiv/internal/logger"
)

type Controller interface {
	Start() error
	NextWallpaper() error
	PauseRotation()
	ResumeRotation()
	OpenCurrentArtwork() error
	Shutdown()
}

// Run starts the tray event loop and wires it to the application controller.
func Run(appCtx context.Context, controller Controller) {
	systray.Run(func() { onReady(appCtx, controller) }, func() {
		controller.Shutdown()
	})
}

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
	openCurrent := systray.AddMenuItem("Open Current Artwork", "Open currently active image")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Quit kPixiv")

	go func() {
		for {
			select {
			case <-next.ClickedCh:
				if err := controller.NextWallpaper(); err != nil {
					log.Warn("Failed to set next wallpaper", "error", err)
				}
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
			case <-openCurrent.ClickedCh:
				if err := controller.OpenCurrentArtwork(); err != nil {
					log.Warn("Failed to open current artwork", "error", err)
				}
			case <-quit.ClickedCh:
				controller.Shutdown()
				systray.Quit()
				return
			case <-appCtx.Done():
				systray.Quit()
				return
			}
		}
	}()
}
