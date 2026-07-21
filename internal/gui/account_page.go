package gui

import (
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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
	description := widget.NewLabel("To connect your Pixiv account:\n\n1. Click the link below to open the Pixiv login page in your browser\n2. Sign in to your Pixiv account\n3. Copy the final redirect URL from the address bar\n4. Paste it in the field below and click Submit")
	description.Wrapping = fyne.TextWrapWord

	openBtn := widget.NewButton("Open Pixiv Login Page", func() {
		//nolint:gosec,errcheck // URL is generated internally; fire-and-forget browser open
		exec.Command("xdg-open", ui.loginURL).Start()
	})

	ui.loginEntry.SetText("")

	cancelBtn := widget.NewButton("Cancel", func() {
		ui.loginInProgress = false
		ui.rebuildAccountPage()
	})

	submitBtn := widget.NewButton("Submit", func() {
		code := ui.loginEntry.Text
		if code == "" {
			return
		}
		go func() {
			if err := ui.ctrl.FinishLogin(code); err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, ui.w)
				})
				return
			}
			fyne.Do(func() {
				ui.loginInProgress = false
				ui.rebuildAccountPage()
			})
		}()
	})

	return container.NewVBox(
		description,
		openBtn,
		ui.loginEntry,
		container.NewHBox(submitBtn, cancelBtn),
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
