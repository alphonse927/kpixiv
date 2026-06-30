package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *settingsUI) buildSettingsPage() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}
	section := func(title string) fyne.CanvasObject {
		return widget.NewLabelWithStyle(title, fyne.TextAlignLeading, bold)
	}
	sideBySide := func(left, right fyne.CanvasObject) fyne.CanvasObject {
		return container.NewGridWithColumns(2, left, right)
	}
	field := func(label string, input fyne.CanvasObject) fyne.CanvasObject {
		return container.NewVBox(
			widget.NewLabel(label),
			input,
		)
	}

	browse := widget.NewButton("Browse...", func() {
		dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				ui.downloadPath.SetText(uri.Path())
			}
		}, ui.w).Show()
	})

	intervals := sideBySide(
		container.NewVBox(
			field("Wallpaper Change (min)", ui.setInterval),
		),
		container.NewVBox(
			field("Download New Images (min)", ui.fetchInterval),
		),
	)

	dims := sideBySide(
		field("Min Width", ui.minWidth),
		field("Min Height", ui.minHeight),
	)

	other := sideBySide(
		container.NewVBox(
			field("History Count", ui.historyLimit),
		),
		container.NewVBox(
			field("Cleanup Age (days)", ui.cleanupDays),
		),
	)

	content := container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				section("Intervals"),
				intervals,
			),
		),
		container.NewPadded(
			container.NewVBox(
				section("Wallpaper Source"),
				ui.ranking,
			),
		),
		container.NewPadded(
			container.NewVBox(
				section("Dimensions"),
				widget.NewLabel("Minimum image size"),
				dims,
			),
		),
		container.NewPadded(
			container.NewVBox(
				section("Storage"),
				widget.NewLabel("Download Directory"),
				container.NewBorder(nil, nil, nil, browse, ui.downloadPath),
				other,
			),
		),
		container.NewPadded(
			container.NewVBox(
				section("Lock Screen"),
				ui.lockScreen,
			),
		),
	)

	return container.NewScroll(container.NewPadded(content))
}
