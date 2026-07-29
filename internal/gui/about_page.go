package gui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/alphonse927/kpixiv/internal/build"
)

func (ui *settingsUI) buildAboutPage() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}
	gitURL, _ := url.Parse("https://github.com/alphonse927/kpixiv") //nolint:errcheck // hardcoded URL

	content := container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				widget.NewLabelWithStyle("kPixiv", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewLabelWithStyle("Version "+build.Version, fyne.TextAlignCenter, fyne.TextStyle{}),
			),
		),
		container.NewPadded(
			container.NewVBox(
				widget.NewLabelWithStyle("License", fyne.TextAlignLeading, bold),
				widget.NewLabel("MIT License"),
				widget.NewLabel("Copyright (c) 2026 KPixiv"),
			),
		),
		container.NewPadded(
			container.NewHBox(
				layout.NewSpacer(),
				widget.NewHyperlink("GitHub Repository", gitURL),
				layout.NewSpacer(),
			),
		),
		container.NewPadded(
			container.NewVBox(
				widget.NewLabelWithStyle("Credits", fyne.TextAlignLeading, bold),
				widget.NewLabel("kPixiv uses the following open source projects:"),
				widget.NewLabel("• Fyne - GUI toolkit"),
				widget.NewLabel("• Cobra - CLI framework"),
				widget.NewLabel("• Pixiv for Muzei - Reference project"),
				widget.NewLabel("• Variety - Reference project"),
			),
		),
	)

	return container.NewScroll(container.NewPadded(content))
}
