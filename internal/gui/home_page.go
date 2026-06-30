package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (ui *settingsUI) buildHomePage() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}
	section := func(title string) fyne.CanvasObject {
		return widget.NewLabelWithStyle(title, fyne.TextAlignLeading, bold)
	}

	wallpaperPanel := container.NewBorder(
		nil,
		nil,
		container.NewHBox(ui.statusThumbnail),
		nil,
		ui.statusWallpaper,
	)

	content := container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				section("Current Wallpaper"),
				wallpaperPanel,
			),
		),
		container.NewPadded(
			container.NewVBox(
				section("Quick Statistics"),
				ui.statusCached,
				ui.statusNextRot,
				ui.statusLastRot,
			),
		),
	)

	return container.NewScroll(container.NewPadded(content))
}
