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
	RestartRotation(ctx context.Context)
	OpenCurrentArtwork() error
	OpenFolder() error
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
	openFolder := systray.AddMenuItem("Open Folder", "Open wallpaper directory")
	systray.AddSeparator()
	restart := systray.AddMenuItem("Restart Rotation", "Reset rotation loop")
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
			case <-openFolder.ClickedCh:
				if err := controller.OpenFolder(); err != nil {
					log.Warn("Failed to open folder", "error", err)
				}
			case <-restart.ClickedCh:
				controller.RestartRotation(appCtx)
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
