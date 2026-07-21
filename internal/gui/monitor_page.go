package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (ui *settingsUI) buildMonitorPage() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}
	description := widget.NewLabel("Enable independent wallpaper rotation and choose the preferred image orientation for each active monitor.")
	description.Wrapping = fyne.TextWrapWord
	return container.NewScroll(container.NewPadded(container.NewVBox(
		widget.NewLabelWithStyle("Monitor Wallpapers", fyne.TextAlignLeading, bold),
		description,
		ui.multiMonitor,
		widget.NewSeparator(),
		ui.monitorSettings,
	)))
}
