package gui

import (
	"fmt"
	"image/color"
	"log/slog"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/alphonse927/kpixiv/internal/assets"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

type numericalEntry struct {
	widget.Entry
}

func newNumericalEntry() *numericalEntry {
	e := &numericalEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *numericalEntry) TypedRune(r rune) {
	if r >= '0' && r <= '9' {
		e.Entry.TypedRune(r)
	}
}

func (e *numericalEntry) TypedShortcut(shortcut fyne.Shortcut) {
	paste, ok := shortcut.(*fyne.ShortcutPaste)
	if !ok {
		e.Entry.TypedShortcut(shortcut)
		return
	}
	content := paste.Clipboard.Content()
	if _, err := strconv.Atoi(content); err == nil {
		e.Entry.TypedShortcut(shortcut)
	}
}

func (e *numericalEntry) Keyboard() mobile.KeyboardType {
	return mobile.NumberKeyboard
}

type tintedBG struct {
	fyne.Theme
}

func (t *tintedBG) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		if variant == theme.VariantDark {
			return color.RGBA{R: 45, G: 52, B: 68, A: 255}
		}
		return color.RGBA{R: 220, G: 228, B: 245, A: 255}
	}
	return t.Theme.Color(name, variant)
}

type settingsUI struct {
	w    fyne.Window
	log  *slog.Logger
	ctrl AppController

	navButtons    []*widget.Button
	content       *fyne.Container
	pages         []fyne.CanvasObject
	currentPage   int
	statusRefresh chan struct{}

	downloadPath  *widget.Entry
	setInterval   *numericalEntry
	fetchInterval *numericalEntry
	historyLimit  *numericalEntry
	cleanupDays   *numericalEntry
	ranking       *widget.Select
	minWidth      *numericalEntry
	minHeight     *numericalEntry
	lockScreen    *widget.Check

	bookmarksEnabled      *widget.Check
	bookmarksSyncInterval *numericalEntry
	bookmarksAutoCleanup  *widget.Check

	statusWallpaper *widget.Label
	statusCached    *widget.Label
	statusLastRot   *widget.Label
	statusNextRot   *widget.Label
	statusThumbnail *canvas.Image
	lastWallpaperID string

	autostartCheck     *widget.Check
	autostartStatus    *widget.Label
	autostartOrigState bool

	accountStatus    *widget.Label
	accountLoginBtn  *widget.Button
	accountLogoutBtn *widget.Button
}

func newSettingsUI(a fyne.App, ctrl AppController, log *slog.Logger) *settingsUI {
	ui := &settingsUI{
		log:  log,
		ctrl: ctrl,
	}
	ui.build(a)
	return ui
}

func (ui *settingsUI) build(a fyne.App) {
	w := a.NewWindow("kPixiv – Settings")
	w.Resize(fyne.NewSize(640, 520))
	w.SetFixedSize(false)
	w.CenterOnScreen()
	w.SetIcon(fyne.NewStaticResource("kpixiv", assets.IconPNG))
	ui.w = w
	w.SetCloseIntercept(ui.hide)

	ui.createWidgets()
	w.SetContent(ui.buildLayout())
}

func (ui *settingsUI) createWidgets() {
	cfg := ui.ctrl.Config()

	ui.downloadPath = widget.NewEntry()
	ui.downloadPath.SetText(cfg.DownloadPath)
	ui.downloadPath.PlaceHolder = "~/Pictures/KPixiv"

	ui.setInterval = newNumericalEntry()
	ui.setInterval.SetText(strconv.Itoa(cfg.Wallpaper.SetInterval))

	ui.fetchInterval = newNumericalEntry()
	ui.fetchInterval.SetText(strconv.Itoa(cfg.Wallpaper.FetchInterval))

	ui.historyLimit = newNumericalEntry()
	ui.historyLimit.SetText(strconv.Itoa(cfg.Wallpaper.HistoryLimit))

	ui.cleanupDays = newNumericalEntry()
	ui.cleanupDays.SetText(strconv.Itoa(cfg.Wallpaper.CleanupDays))

	ui.ranking = widget.NewSelect([]string{"daily", "weekly", "monthly"}, nil)
	ui.ranking.SetSelected(cfg.Pixiv.Ranking.String())

	ui.minWidth = newNumericalEntry()
	ui.minWidth.SetText(strconv.Itoa(cfg.Pixiv.MinWidth))

	ui.minHeight = newNumericalEntry()
	ui.minHeight.SetText(strconv.Itoa(cfg.Pixiv.MinHeight))

	ui.lockScreen = widget.NewCheck("Set Lock Screen", nil)
	ui.lockScreen.SetChecked(cfg.KDE.SetLockScreen)

	ui.bookmarksEnabled = widget.NewCheck("Enable Bookmark Sync", func(enabled bool) {
		if enabled {
			ui.bookmarksSyncInterval.Enable()
			ui.bookmarksAutoCleanup.Enable()
		} else {
			ui.bookmarksSyncInterval.Disable()
			ui.bookmarksAutoCleanup.Disable()
		}
	})
	ui.bookmarksEnabled.SetChecked(cfg.Bookmarks.Enabled)

	ui.bookmarksSyncInterval = newNumericalEntry()
	ui.bookmarksSyncInterval.SetText(strconv.Itoa(cfg.Bookmarks.SyncInterval))

	ui.bookmarksAutoCleanup = widget.NewCheck("Remove unbookmarked images", nil)
	ui.bookmarksAutoCleanup.SetChecked(cfg.Bookmarks.AutoCleanup)

	if !cfg.Bookmarks.Enabled {
		ui.bookmarksSyncInterval.Disable()
		ui.bookmarksAutoCleanup.Disable()
	}

	ui.statusWallpaper = widget.NewLabel("")
	ui.statusWallpaper.Wrapping = fyne.TextWrapWord
	ui.statusCached = widget.NewLabel("")
	ui.statusLastRot = widget.NewLabel("")
	ui.statusNextRot = widget.NewLabel("")

	ui.statusThumbnail = canvas.NewImageFromFile("")
	ui.statusThumbnail.FillMode = canvas.ImageFillContain
	ui.statusThumbnail.SetMinSize(fyne.NewSize(140, 0))

	ui.autostartCheck = widget.NewCheck("Start KPixiv automatically when I log in", nil)
	ui.autostartCheck.Disable()
	ui.autostartStatus = widget.NewLabel("")
	ui.autostartStatus.Hide()

	ui.accountStatus = widget.NewLabel("")
	ui.accountLoginBtn = widget.NewButton("Login to Pixiv", func() {
		go func() {
			if err := ui.ctrl.LoginToPixiv(); err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, ui.w)
				})
			}
			fyne.Do(func() {
				ui.rebuildAccountPage()
			})
		}()
	})
	ui.accountLogoutBtn = widget.NewButton("Logout", func() {
		if err := ui.ctrl.LogoutFromPixiv(); err != nil {
			dialog.ShowError(err, ui.w)
			return
		}
		ui.rebuildAccountPage()
	})
}

func (ui *settingsUI) buildLayout() fyne.CanvasObject {
	sidebar := container.NewVBox()

	navDefs := []struct {
		label string
		build func() fyne.CanvasObject
	}{
		{"🏠 Home", ui.buildHomePage},
		{"⚙️ Settings", ui.buildSettingsPage},
		{"👤 Account", ui.buildAccountPage},
		{"ℹ️ About", ui.buildAboutPage},
	}

	ui.navButtons = make([]*widget.Button, len(navDefs))
	ui.pages = make([]fyne.CanvasObject, len(navDefs))

	for i, def := range navDefs {
		idx := i

		ui.pages[i] = def.build()

		btn := widget.NewButton(def.label, func() {
			ui.selectPage(idx)
		})

		ui.navButtons[i] = btn
		sidebar.Add(btn)
	}

	sidebar.Add(layout.NewSpacer())

	sidebarBox := container.NewPadded(sidebar)

	ui.content = container.NewStack()

	for _, page := range ui.pages {
		ui.content.Add(page)
	}

	for i := 1; i < len(ui.pages); i++ {
		ui.pages[i].Hide()
	}

	ui.currentPage = 0
	ui.highlightNav(0)

	bottomBar := container.NewPadded(
		container.NewHBox(
			layout.NewSpacer(),
			widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), ui.hide),
			widget.NewButton("Apply", ui.applySettings),
			widget.NewButtonWithIcon("Save & Close", theme.ConfirmIcon(), func() {
				ui.applySettings()
				ui.log.Info("Settings saved and closed")
				ui.hide()
			}),
		),
	)

	mainContent := container.New(
		NewFixedWidthLayout(125),
		sidebarBox,
		container.NewPadded(ui.content),
	)

	return container.NewBorder(
		nil,
		bottomBar,
		nil,
		nil,
		mainContent,
	)
}

func (ui *settingsUI) selectPage(idx int) {
	if ui.currentPage == idx {
		return
	}

	ui.pages[ui.currentPage].Hide()
	ui.pages[idx].Show()
	ui.currentPage = idx

	ui.highlightNav(idx)
}

func (ui *settingsUI) highlightNav(idx int) {
	for i, btn := range ui.navButtons {
		if i == idx {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.MediumImportance
		}
		btn.Refresh()
	}
}

func (ui *settingsUI) rebuildAccountPage() {
	old := ui.pages[2]
	ui.pages[2] = ui.buildAccountPage()
	ui.content.Remove(old)
	ui.content.Add(ui.pages[2])
	if ui.currentPage != 2 {
		ui.pages[2].Hide()
	}
}

func (ui *settingsUI) show() {
	ui.refreshStatus()
	ui.w.Show()
	ui.startStatusRefresh()
	go ui.refreshAutostartState()
}

func (ui *settingsUI) hide() {
	ui.stopStatusRefresh()
	ui.log.Debug("Settings window closed")
	ui.w.Hide()
}

func (ui *settingsUI) update() {
	cfg := ui.ctrl.Config()
	ui.downloadPath.SetText(cfg.DownloadPath)
	ui.setInterval.SetText(strconv.Itoa(cfg.Wallpaper.SetInterval))
	ui.fetchInterval.SetText(strconv.Itoa(cfg.Wallpaper.FetchInterval))
	ui.historyLimit.SetText(strconv.Itoa(cfg.Wallpaper.HistoryLimit))
	ui.cleanupDays.SetText(strconv.Itoa(cfg.Wallpaper.CleanupDays))
	ui.ranking.SetSelected(cfg.Pixiv.Ranking.String())
	ui.minWidth.SetText(strconv.Itoa(cfg.Pixiv.MinWidth))
	ui.minHeight.SetText(strconv.Itoa(cfg.Pixiv.MinHeight))
	ui.lockScreen.SetChecked(cfg.KDE.SetLockScreen)

	ui.bookmarksEnabled.SetChecked(cfg.Bookmarks.Enabled)
	ui.bookmarksSyncInterval.SetText(strconv.Itoa(cfg.Bookmarks.SyncInterval))
	ui.bookmarksAutoCleanup.SetChecked(cfg.Bookmarks.AutoCleanup)

	if cfg.Bookmarks.Enabled {
		ui.bookmarksSyncInterval.Enable()
		ui.bookmarksAutoCleanup.Enable()
	} else {
		ui.bookmarksSyncInterval.Disable()
		ui.bookmarksAutoCleanup.Disable()
	}
}

func (ui *settingsUI) applySettings() {
	cfg := ui.ctrl.Config()
	cfg.DownloadPath = ui.downloadPath.Text

	if v, err := strconv.Atoi(ui.setInterval.Text); err == nil {
		cfg.Wallpaper.SetInterval = v
	}

	if v, err := strconv.Atoi(ui.fetchInterval.Text); err == nil {
		cfg.Wallpaper.FetchInterval = v
	}

	if v, err := strconv.Atoi(ui.historyLimit.Text); err == nil {
		cfg.Wallpaper.HistoryLimit = v
	}

	if v, err := strconv.Atoi(ui.cleanupDays.Text); err == nil {
		cfg.Wallpaper.CleanupDays = v
	}

	if v, err := strconv.Atoi(ui.minWidth.Text); err == nil {
		cfg.Pixiv.MinWidth = v
	}

	if v, err := strconv.Atoi(ui.minHeight.Text); err == nil {
		cfg.Pixiv.MinHeight = v
	}

	switch ui.ranking.Selected {
	case "weekly":
		cfg.Pixiv.Ranking = config.RankingWeeklyMode
	case "monthly":
		cfg.Pixiv.Ranking = config.RankingMonthlyMode
	default:
		cfg.Pixiv.Ranking = config.RankingDailyMode
	}

	cfg.KDE.SetLockScreen = ui.lockScreen.Checked

	if ui.lockScreen.Checked {
		updater := wallpaper.NewKDELockScreenUpdater()
		if err := updater.EnsureConfigExists(); err != nil {
			dialog.ShowError(fmt.Errorf("failed to prepare lock screen config: %w", err), ui.w)
			return
		}
	}

	cfg.Bookmarks.Enabled = ui.bookmarksEnabled.Checked
	cfg.Bookmarks.AutoCleanup = ui.bookmarksAutoCleanup.Checked
	if v, err := strconv.Atoi(ui.bookmarksSyncInterval.Text); err == nil {
		cfg.Bookmarks.SyncInterval = v
	}

	cfg.Validate()

	if err := config.Save(cfg.ConfigPath, cfg); err != nil {
		dialog.ShowError(err, ui.w)
		return
	}

	ui.ctrl.ApplyConfig(cfg)

	ui.applyAutostart()

	ui.log.Info("Settings applied")
}

func (ui *settingsUI) applyAutostart() {
	if ui.autostartCheck.Checked == ui.autostartOrigState {
		return
	}
	var err error
	if ui.autostartCheck.Checked {
		err = ui.ctrl.EnableService()
	} else {
		err = ui.ctrl.DisableService()
	}
	if err != nil {
		ui.autostartCheck.SetChecked(ui.autostartOrigState)
		dialog.ShowError(err, ui.w)
		return
	}
	ui.autostartOrigState = ui.autostartCheck.Checked
}

func (ui *settingsUI) startStatusRefresh() {
	if ui.statusRefresh != nil {
		return
	}
	ui.statusRefresh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fyne.Do(ui.refreshStatus)
			case <-ui.statusRefresh:
				return
			}
		}
	}()
}

func (ui *settingsUI) stopStatusRefresh() {
	if ui.statusRefresh != nil {
		close(ui.statusRefresh)
		ui.statusRefresh = nil
	}
}

func (ui *settingsUI) refreshStatus() {
	meta, _ := ui.ctrl.CurrentWallpaper() //nolint:errcheck

	if meta != nil && meta.ID != "" && meta.ID != ui.lastWallpaperID {
		ui.lastWallpaperID = meta.ID
		thumbPath := ui.ctrl.ThumbnailPath(meta.ID)
		ui.statusThumbnail.File = thumbPath
		ui.statusThumbnail.Refresh()
	}

	if meta != nil && meta.ID != "" && meta.Title != "" {
		ui.statusWallpaper.SetText(ui.formatWallpaperInfo(meta))
		ui.statusThumbnail.Show()
	} else {
		ui.statusWallpaper.SetText("No wallpaper set")
		ui.statusThumbnail.Hide()
	}

	ui.statusCached.SetText(ui.formatCachedCount())
	ui.statusLastRot.SetText(ui.formatLastRotation())
	ui.statusNextRot.SetText(ui.formatNextRotation())
}

func (ui *settingsUI) refreshAutostartState() {
	enabled, err := ui.ctrl.ServiceEnabled()
	fyne.Do(func() {
		if err != nil {
			ui.autostartCheck.Disable()
			ui.autostartCheck.SetChecked(false)
			ui.autostartStatus.SetText("Unable to access the systemd user service.")
			ui.autostartStatus.Show()
			return
		}
		ui.autostartCheck.Enable()
		ui.autostartCheck.SetChecked(enabled)
		ui.autostartOrigState = enabled
		ui.autostartStatus.Hide()
	})
}

func (ui *settingsUI) formatWallpaperInfo(meta *storage.ImageMeta) string {
	res := ""
	if meta.Width > 0 && meta.Height > 0 {
		res = strconv.Itoa(meta.Width) + " × " + strconv.Itoa(meta.Height)
	}

	src := formatSource(meta.Source, meta.Rank)
	return meta.Title + "\n" + meta.Artist + "\n" + res + "\n" + src
}

func (ui *settingsUI) formatCachedCount() string {
	return "Cached wallpapers: " + strconv.Itoa(ui.ctrl.CachedCount())
}

func (ui *settingsUI) formatLastRotation() string {
	t := ui.ctrl.LastRotation()
	if t.IsZero() {
		return "Last rotation: Never"
	}
	return "Last rotation: " + t.Format("Jan 02, 15:04")
}

func (ui *settingsUI) formatNextRotation() string {
	lastRot := ui.ctrl.LastRotation()
	if lastRot.IsZero() {
		return "Next change: No rotation scheduled"
	}
	cfg := ui.ctrl.Config()
	interval := time.Duration(cfg.Wallpaper.SetInterval) * time.Minute
	next := lastRot.Add(interval)
	remaining := time.Until(next)
	if remaining <= 0 {
		return "Next change: Any moment now"
	}
	mins := int(remaining.Minutes())
	secs := int(remaining.Seconds()) % 60
	if mins > 0 {
		return "Next change: in " + strconv.Itoa(mins) + "m " + strconv.Itoa(secs) + "s"
	}
	return "Next change: in " + strconv.Itoa(secs) + "s"
}

func formatSource(source string, rank int) string {
	switch source {
	case "daily":
		if rank > 0 {
			return "Daily Ranking (#" + strconv.Itoa(rank) + ")"
		}
		return "Daily Ranking"
	case "weekly":
		if rank > 0 {
			return "Weekly Ranking (#" + strconv.Itoa(rank) + ")"
		}
		return "Weekly Ranking"
	case "monthly":
		if rank > 0 {
			return "Monthly Ranking (#" + strconv.Itoa(rank) + ")"
		}
		return "Monthly Ranking"
	case "favorites":
		return "Bookmarks"
	default:
		if source != "" {
			return source
		}
		return "Unknown"
	}
}
