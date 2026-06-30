package gui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/storage"
)

type AppController interface {
	Config() *config.Config
	ApplyConfig(cfg *config.Config)
	NextWallpaper() error
	FetchNow() error
	PixivLoggedIn() bool
	PixivUserName() string
	LoginToPixiv() error
	LogoutFromPixiv() error
	SchedulerRunning() bool
	CurrentWallpaper() (*storage.ImageMeta, error)
	CachedCount() int
	LastRotation() time.Time
}

var (
	guiApp    fyne.App
	settingsW *settingsUI
)

func Run(ctrl AppController, ctx context.Context, quitCh <-chan struct{}) {
	a := app.NewWithID("kpixiv")
	a.Settings().SetTheme(&tintedBG{theme.DefaultTheme()})
	guiApp = a

	settingsW = newSettingsUI(a, ctrl, logger.WithComponent("settings"))

	go func() {
		select {
		case <-quitCh:
		case <-ctx.Done():
		}
		fyne.Do(a.Quit)
	}()

	a.Run()
}

func ShowSettings(ctrl AppController) {
	if guiApp == nil {
		return
	}
	fyne.Do(func() {
		settingsW.ctrl = ctrl
		settingsW.update()
		settingsW.show()
	})
}
