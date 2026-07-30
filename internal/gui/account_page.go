package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (ui *settingsUI) buildAccountPage() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}
	section := func(title string) fyne.CanvasObject {
		return widget.NewLabelWithStyle(title, fyne.TextAlignLeading, bold)
	}

	content := container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				section("Authentication"),
				ui.buildAccountAuthSection(),
			),
		),
		container.NewPadded(
			container.NewVBox(
				section("Bookmark Sync"),
				ui.buildBookmarkSection(),
			),
		),
	)

	return container.NewScroll(container.NewPadded(content))
}

func (ui *settingsUI) buildAccountAuthSection() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}

	loggedIn := ui.ctrl.PixivLoggedIn()

	if loggedIn {
		username := ui.ctrl.PixivUserName()
		userLabel := "Unknown"
		if username != "" {
			userLabel = username
		}

		return container.NewVBox(
			widget.NewLabelWithStyle("Status", fyne.TextAlignLeading, bold),
			widget.NewLabel("Connected to Pixiv"),
			widget.NewLabelWithStyle("Account", fyne.TextAlignLeading, bold),
			widget.NewLabelWithStyle(userLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(ui.accountLogoutBtn),
		)
	}

	if ui.loginInProgress {
		return ui.buildLoginForm()
	}

	return container.NewVBox(
		widget.NewLabelWithStyle("Status", fyne.TextAlignLeading, bold),
		widget.NewLabel("Not connected"),
		container.NewHBox(ui.accountLoginBtn),
	)
}

func (ui *settingsUI) buildLoginForm() fyne.CanvasObject {
	ui.loginStatus.SetText("Opening browser...")

	cancelBtn := widget.NewButton("Cancel", func() {
		if ui.loginCancel != nil {
			ui.loginCancel()
		}
		ui.loginInProgress = false
		ui.rebuildAccountPage()
	})

	return container.NewVBox(
		ui.loginStatus,
		cancelBtn,
	)
}

func (ui *settingsUI) buildBookmarkSection() fyne.CanvasObject {
	field := func(label string, input fyne.CanvasObject) fyne.CanvasObject {
		return container.NewVBox(
			widget.NewLabel(label),
			input,
		)
	}

	return container.NewVBox(
		ui.bookmarksEnabled,
		widget.NewLabel("Sync bookmarks from Pixiv"),
		field("Sync Interval (min)", ui.bookmarksSyncInterval),
		widget.NewLabel("Minimum 60 minutes"),
		ui.bookmarksAutoCleanup,
	)
}
