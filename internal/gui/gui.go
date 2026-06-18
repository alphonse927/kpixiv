package gui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
)

// OnApply is called after settings are saved to disk.
type OnApply func()

var (
	guiApp    fyne.App
	settingsW *settingsUI
)

// Run creates the Fyne app and settings window, then runs the
// event loop on the calling goroutine (must be the main goroutine).
// Blocks until quitCh is closed or ctx is cancelled.
func Run(cfg *config.Config, onApply OnApply, ctx context.Context, quitCh <-chan struct{}) {
	a := app.NewWithID("kpixiv")
	a.Settings().SetTheme(&tintedBG{theme.DefaultTheme()})
	guiApp = a

	settingsW = newSettingsUI(a, cfg, logger.WithComponent("settings"), onApply)

	go func() {
		select {
		case <-quitCh:
		case <-ctx.Done():
		}
		fyne.Do(a.Quit)
	}()

	a.Run()
}

// ShowSettings updates the settings window with the current config
// and shows it. Safe to call from any goroutine — dispatches via fyne.Do.
func ShowSettings(cfg *config.Config, onApply OnApply) {
	if guiApp == nil {
		return
	}
	fyne.Do(func() {
		settingsW.update(cfg)
		settingsW.onApply = onApply
		settingsW.show()
	})
}
