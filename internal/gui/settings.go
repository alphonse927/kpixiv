package gui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"strconv"
	"strings"
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
	"github.com/alphonse927/kpixiv/internal/auth"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

const (
	feedSourceDailyRanking   = "Daily Ranking"
	feedSourceWeeklyRanking  = "Weekly Ranking"
	feedSourceMonthlyRanking = "Monthly Ranking"
	feedSourceBookmarks      = "Bookmarks"
	feedSourceAll            = "All"
)

const (
	rankingSubDaily   = "Daily"
	rankingSubWeekly  = "Weekly"
	rankingSubMonthly = "Monthly"
)

var orientationLabels = []string{
	config.WallpaperAnyOrientation.Label(),
	config.WallpaperLandscapeOrientation.Label(),
	config.WallpaperPortraitOrientation.Label(),
}

type numericalEntry struct {
	widget.Entry
}

func newNumericalEntry() *numericalEntry {
	e := &numericalEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func newNumericalWithText(text string) *numericalEntry {
	e := newNumericalEntry()
	e.SetText(text)
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
		if variant == theme.VariantLight {
			return color.RGBA{R: 220, G: 228, B: 245, A: 255}
		}
		return color.RGBA{R: 45, G: 52, B: 68, A: 255}
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

	downloadPath         *widget.Entry
	setInterval          *numericalEntry
	fetchInterval        *numericalEntry
	historyLimit         *numericalEntry
	cleanupDays          *numericalEntry
	feedSource           *widget.Select
	rankingSub           *widget.Select
	rotationEnabled      *widget.Check
	fetchEnabled         *widget.Check
	wallpaperOrientation *widget.Select
	minWidth             *numericalEntry
	minHeight            *numericalEntry
	lockScreen           *widget.Check
	multiMonitor         *widget.Check
	monitorSettings      *fyne.Container
	monitorChecks        map[string]*widget.Check
	monitorOrientations  map[string]*widget.Select

	logLevel *widget.Select

	notificationsEnabled *widget.Check

	bookmarksEnabled      *widget.Check
	bookmarksSyncInterval *numericalEntry
	bookmarksAutoCleanup  *widget.Check

	statusWallpaper         *widget.Label
	statusCached            *widget.Label
	statusLastRot           *widget.Label
	statusNextRot           *widget.Label
	statusThumbnail         *canvas.Image
	monitorStatus           *fyne.Container
	currentWallpaperSection *fyne.Container
	monitorWallpaperSection *fyne.Container
	lastWallpaperID         string

	statusDaemon    *widget.Label
	statusDaemonDot *canvas.Circle
	statusAuth      *widget.Label
	statusAuthDot   *canvas.Circle
	statusCache     *widget.Label
	statusHistory   *widget.Label
	statusNextWall  *widget.Label
	statusWarnings  *widget.Label
	firstRunBox     *fyne.Container
	firstRunTitle   *widget.Label
	firstRunDetail  *widget.Label
	firstRunActions *fyne.Container

	statusLastFetch        *widget.Label
	statusNextFetch        *widget.Label
	statusLastBookmarkSync *widget.Label
	statusNextBookmarkSync *widget.Label
	bookmarkSyncRows       *fyne.Container

	autostartCheck     *widget.Check
	autostartStatus    *widget.Label
	autostartOrigState bool

	accountStatus    *widget.Label
	accountLoginBtn  *widget.Button
	accountLogoutBtn *widget.Button
	loginInProgress  bool
	loginStatus      *widget.Label
	loginURL         *widget.Entry
	loginCancel      context.CancelFunc
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
	w.Resize(fyne.NewSize(720, 600))
	w.SetFixedSize(false)
	w.CenterOnScreen()
	w.SetIcon(fyne.NewStaticResource("kpixiv", assets.IconPNG))
	ui.w = w
	w.SetCloseIntercept(ui.hide)

	ui.createWidgets()
	w.SetContent(ui.buildLayout())
}

func (ui *settingsUI) createDashboardStatusWidgets() {
	ui.statusDaemon = widget.NewLabel("")
	ui.statusDaemonDot = canvas.NewCircle(color.Transparent)
	ui.statusAuth = widget.NewLabel("")
	ui.statusAuthDot = canvas.NewCircle(color.Transparent)
	ui.statusCache = widget.NewLabel("")
	ui.statusHistory = widget.NewLabel("")
	ui.statusNextWall = widget.NewLabel("")
	ui.statusNextWall.Wrapping = fyne.TextWrapWord
	ui.statusWarnings = widget.NewLabel("")
	ui.statusWarnings.Wrapping = fyne.TextWrapWord
	ui.statusWarnings.Hide()

	ui.statusLastFetch = widget.NewLabel("")
	ui.statusNextFetch = widget.NewLabel("")
	ui.statusLastBookmarkSync = widget.NewLabel("")
	ui.statusNextBookmarkSync = widget.NewLabel("")
}

func (ui *settingsUI) createWidgets() {
	cfg := ui.ctrl.Config()

	ui.downloadPath = widget.NewEntry()
	ui.downloadPath.SetText(cfg.DownloadPath)
	ui.downloadPath.PlaceHolder = "~/Pictures/KPixiv"

	ui.setInterval = newNumericalWithText(strconv.Itoa(cfg.Wallpaper.SetInterval))
	ui.fetchInterval = newNumericalWithText(strconv.Itoa(cfg.Wallpaper.FetchInterval))
	ui.historyLimit = newNumericalWithText(strconv.Itoa(cfg.Wallpaper.HistoryLimit))
	ui.cleanupDays = newNumericalWithText(strconv.Itoa(cfg.Wallpaper.CleanupDays))

	ui.rotationEnabled = widget.NewCheck("Enable Wallpaper Rotation", nil)
	ui.rotationEnabled.SetChecked(cfg.Wallpaper.RotationEnabled)

	ui.fetchEnabled = widget.NewCheck("Enable Ranking Fetch", nil)
	ui.fetchEnabled.SetChecked(cfg.Wallpaper.FetchEnabled)

	ui.createWallpaperOrientationWidget(cfg)

	ui.rankingSub = widget.NewSelect([]string{rankingSubDaily, rankingSubWeekly, rankingSubMonthly}, nil)
	ui.rankingSub.SetSelected(rankingSubDisplay(cfg.Pixiv.Ranking))
	if cfg.Wallpaper.QueueSource != config.QueueSourceAll {
		ui.rankingSub.Hide()
	}

	feedOptions := []string{feedSourceDailyRanking, feedSourceWeeklyRanking, feedSourceMonthlyRanking, feedSourceBookmarks, feedSourceAll}
	ui.feedSource = widget.NewSelect(feedOptions, func(selected string) {
		if selected == feedSourceAll {
			ui.rankingSub.Show()
		} else {
			ui.rankingSub.Hide()
		}
	})
	ui.feedSource.SetSelected(feedSourceDisplay(cfg))

	ui.minWidth = newNumericalWithText(strconv.Itoa(cfg.Pixiv.MinWidth))
	ui.minHeight = newNumericalWithText(strconv.Itoa(cfg.Pixiv.MinHeight))

	ui.lockScreen = widget.NewCheck("Set Lock Screen", nil)
	ui.lockScreen.SetChecked(cfg.KDE.SetLockScreen)
	ui.createMonitorWidgets()

	ui.bookmarksSyncInterval = newNumericalWithText(strconv.Itoa(cfg.Bookmarks.SyncInterval))

	ui.bookmarksAutoCleanup = widget.NewCheck("Remove unbookmarked images", nil)
	ui.bookmarksAutoCleanup.SetChecked(cfg.Bookmarks.AutoCleanup)

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

	if !cfg.Bookmarks.Enabled {
		ui.bookmarksSyncInterval.Disable()
		ui.bookmarksAutoCleanup.Disable()
	}

	ui.statusWallpaper = widget.NewLabel("")
	ui.statusWallpaper.Wrapping = fyne.TextWrapWord
	ui.statusCached = widget.NewLabel("")
	ui.statusLastRot = widget.NewLabel("")
	ui.statusNextRot = widget.NewLabel("")
	ui.createDashboardStatusWidgets()

	ui.statusThumbnail = canvas.NewImageFromFile("")
	ui.statusThumbnail.FillMode = canvas.ImageFillContain
	ui.statusThumbnail.SetMinSize(fyne.NewSize(140, 0))
	ui.monitorStatus = container.NewVBox()

	ui.logLevel = widget.NewSelect([]string{"info", "debug"}, nil)
	ui.logLevel.SetSelected(cfg.LogLevel)

	ui.notificationsEnabled = widget.NewCheck("Show desktop notifications", nil)
	ui.notificationsEnabled.SetChecked(cfg.Notifications.Enabled)

	ui.autostartCheck = widget.NewCheck("Run KPixiv in the background (starts on login, via systemd)", nil)
	ui.autostartCheck.Disable()
	ui.autostartStatus = widget.NewLabel("")
	ui.autostartStatus.Hide()

	ui.accountStatus = widget.NewLabel("")
	ui.loginStatus = widget.NewLabel("")
	ui.loginStatus.Wrapping = fyne.TextWrapWord
	ui.loginURL = widget.NewEntry()
	ui.loginURL.Hide()
	ui.loginURL.Disable()
	ui.loginURL.Wrapping = fyne.TextWrapWord
	ui.accountLoginBtn = ui.newLoginButton()
	ui.accountLogoutBtn = ui.newLogoutButton()
}

func (ui *settingsUI) newLoginButton() *widget.Button {
	return widget.NewButton("Login to Pixiv", func() {
		ui.loginInProgress = true
		ui.rebuildAccountPage()
		ctx, cancel := context.WithCancel(context.Background())
		ui.loginCancel = cancel
		loginCfg := auth.LoginConfig{
			OnAuthURL: func(url string) {
				fyne.Do(func() {
					ui.loginStatus.SetText("Waiting for authentication...")
					ui.loginURL.SetText(url)
					ui.loginURL.Show()
				})
			},
		}
		go func() {
			err := ui.ctrl.AutoLogin(ctx, loginCfg)
			fyne.Do(func() {
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						dialog.ShowError(err, ui.w)
					}
					ui.loginInProgress = false
					ui.loginCancel = nil
					ui.rebuildAccountPage()
					return
				}
				ui.loginInProgress = false
				ui.loginCancel = nil
				ui.rebuildAccountPage()
			})
		}()
	})
}

func (ui *settingsUI) newLogoutButton() *widget.Button {
	return widget.NewButton("Logout", func() {
		if err := ui.ctrl.LogoutFromPixiv(); err != nil {
			dialog.ShowError(err, ui.w)
			return
		}
		ui.rebuildAccountPage()
	})
}

func (ui *settingsUI) createWallpaperOrientationWidget(cfg *config.Config) {
	ui.wallpaperOrientation = widget.NewSelect(orientationLabels, nil)
	ui.wallpaperOrientation.SetSelected(orientationDisplay(cfg.Wallpaper.Orientation))
}

func (ui *settingsUI) createMonitorWidgets() {
	cfg := ui.ctrl.Config()
	ui.multiMonitor = widget.NewCheck("Enable independent wallpapers for each monitor", func(enabled bool) {
		ui.setMonitorControlsEnabled(enabled)
	})
	ui.multiMonitor.SetChecked(cfg.Wallpaper.MultiMonitorEnabled)
	ui.monitorChecks = make(map[string]*widget.Check)
	ui.monitorOrientations = make(map[string]*widget.Select)
	ui.monitorSettings = container.NewVBox()
	ui.refreshMonitorSettings()
}

func (ui *settingsUI) buildLayout() fyne.CanvasObject {
	sidebar := container.NewVBox()

	navDefs := []struct {
		label string
		icon  fyne.Resource
		build func() fyne.CanvasObject
	}{
		{"Home", theme.HomeIcon(), ui.buildHomePage},
		{"Settings", theme.SettingsIcon(), ui.buildSettingsPage},
		{"Monitors", theme.DesktopIcon(), ui.buildMonitorPage},
		{"Account", theme.AccountIcon(), ui.buildAccountPage},
		{"About", theme.InfoIcon(), ui.buildAboutPage},
	}

	ui.navButtons = make([]*widget.Button, len(navDefs))
	ui.pages = make([]fyne.CanvasObject, len(navDefs))

	for i, def := range navDefs {
		idx := i
		ui.pages[i] = def.build()

		btn := widget.NewButtonWithIcon(def.label, def.icon, func() {
			ui.selectPage(idx)
		})
		btn.Alignment = widget.ButtonAlignLeading

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
			widget.NewButtonWithIcon("OK", theme.ConfirmIcon(), func() {
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
	old := ui.pages[AccountPage]
	ui.pages[AccountPage] = ui.buildAccountPage()
	ui.content.Remove(old)
	ui.content.Add(ui.pages[AccountPage])
	if ui.currentPage != AccountPage {
		ui.pages[AccountPage].Hide()
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
	ui.feedSource.SetSelected(feedSourceDisplay(cfg))
	ui.rankingSub.SetSelected(rankingSubDisplay(cfg.Pixiv.Ranking))
	if cfg.Wallpaper.QueueSource == config.QueueSourceAll {
		ui.rankingSub.Show()
	} else {
		ui.rankingSub.Hide()
	}
	ui.rotationEnabled.SetChecked(cfg.Wallpaper.RotationEnabled)
	ui.fetchEnabled.SetChecked(cfg.Wallpaper.FetchEnabled)
	ui.wallpaperOrientation.SetSelected(orientationDisplay(cfg.Wallpaper.Orientation))
	ui.minWidth.SetText(strconv.Itoa(cfg.Pixiv.MinWidth))
	ui.minHeight.SetText(strconv.Itoa(cfg.Pixiv.MinHeight))
	ui.lockScreen.SetChecked(cfg.KDE.SetLockScreen)
	ui.multiMonitor.SetChecked(cfg.Wallpaper.MultiMonitorEnabled)
	ui.refreshMonitorSettings()

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

	ui.logLevel.SetSelected(cfg.LogLevel)
	ui.notificationsEnabled.SetChecked(cfg.Notifications.Enabled)
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

	cfg.Wallpaper.RotationEnabled = ui.rotationEnabled.Checked
	cfg.Wallpaper.FetchEnabled = ui.fetchEnabled.Checked
	cfg.Wallpaper.Orientation = orientationValue(ui.wallpaperOrientation.Selected)

	if v, err := strconv.Atoi(ui.minWidth.Text); err == nil {
		cfg.Pixiv.MinWidth = v
	}

	if v, err := strconv.Atoi(ui.minHeight.Text); err == nil {
		cfg.Pixiv.MinHeight = v
	}

	ui.applyFeedSource(cfg)

	cfg.KDE.SetLockScreen = ui.lockScreen.Checked
	cfg.Wallpaper.MultiMonitorEnabled = ui.multiMonitor.Checked
	if cfg.Wallpaper.Monitors == nil {
		cfg.Wallpaper.Monitors = map[string]config.MonitorConfig{}
	}
	for id, check := range ui.monitorChecks {
		cfg.Wallpaper.Monitors[id] = config.MonitorConfig{
			RotationEnabled: check.Checked,
			Orientation:     orientationValue(ui.monitorOrientations[id].Selected),
		}
	}

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

	cfg.LogLevel = ui.logLevel.Selected
	cfg.Notifications.Enabled = ui.notificationsEnabled.Checked

	issues := cfg.Validate()
	if len(issues) > 0 {
		dialog.ShowInformation("Configuration adjusted",
			"Some settings were outside the supported range and have been adjusted:\n\n• "+strings.Join(issues, "\n• "),
			ui.w)
	}

	if err := config.Save(cfg.ConfigPath, cfg); err != nil {
		dialog.ShowError(err, ui.w)
		return
	}

	ui.ctrl.ApplyConfig(cfg)

	ui.applyAutostart()

	ui.log.Info("Settings applied")
}

func (ui *settingsUI) applyFeedSource(cfg *config.Config) {
	switch ui.feedSource.Selected {
	case feedSourceWeeklyRanking:
		cfg.Pixiv.Ranking = config.RankingWeeklyMode
		cfg.Wallpaper.QueueSource = config.QueueSourceRanking
	case feedSourceMonthlyRanking:
		cfg.Pixiv.Ranking = config.RankingMonthlyMode
		cfg.Wallpaper.QueueSource = config.QueueSourceRanking
	case feedSourceBookmarks:
		cfg.Wallpaper.QueueSource = config.QueueSourceBookmarks
	case feedSourceAll:
		cfg.Wallpaper.QueueSource = config.QueueSourceAll
		switch ui.rankingSub.Selected {
		case rankingSubWeekly:
			cfg.Pixiv.Ranking = config.RankingWeeklyMode
		case rankingSubMonthly:
			cfg.Pixiv.Ranking = config.RankingMonthlyMode
		default:
			cfg.Pixiv.Ranking = config.RankingDailyMode
		}
	default:
		cfg.Pixiv.Ranking = config.RankingDailyMode
		cfg.Wallpaper.QueueSource = config.QueueSourceRanking
	}
}

func (ui *settingsUI) refreshMonitorSettings() {
	if ui.monitorSettings == nil {
		return
	}

	ui.monitorSettings.Objects = nil
	ui.monitorChecks = make(map[string]*widget.Check)
	ui.monitorOrientations = make(map[string]*widget.Select)
	screens, err := ui.ctrl.Monitors()
	if err != nil || len(screens) == 0 {
		ui.monitorSettings.Add(widget.NewLabel("No active KDE screens detected."))
		ui.monitorSettings.Refresh()
		return
	}

	cfg := ui.ctrl.Config()
	for _, screen := range screens {
		settings, configured := cfg.Wallpaper.Monitors[screen.ID]
		enabled := cfg.Wallpaper.RotationEnabled
		if configured {
			enabled = settings.RotationEnabled
		}

		check := widget.NewCheck(screen.Label(), nil)
		check.OnChanged = func(checked bool) {
			if !checked {
				active := 0
				for _, c := range ui.monitorChecks {
					if c.Checked {
						active++
					}
				}
				if active == 0 {
					check.SetChecked(true)
				}
			}
		}

		check.SetChecked(enabled)
		orientation := widget.NewSelect(orientationLabels, nil)
		selectedOrientation := settings.Orientation
		if selectedOrientation == "" || selectedOrientation == config.WallpaperAnyOrientation {
			selectedOrientation = config.WallpaperAnyOrientation
		}

		orientation.SetSelected(orientationDisplay(selectedOrientation))
		ui.monitorChecks[screen.ID] = check
		ui.monitorOrientations[screen.ID] = orientation
		ui.monitorSettings.Add(container.NewGridWithColumns(2, check, orientation))
	}

	ui.monitorSettings.Refresh()
	ui.setMonitorControlsEnabled(ui.multiMonitor.Checked)
}

func (ui *settingsUI) setMonitorControlsEnabled(enabled bool) {
	if ui.wallpaperOrientation != nil {
		if enabled {
			ui.wallpaperOrientation.Disable()
		} else {
			ui.wallpaperOrientation.Enable()
		}
	}

	for _, check := range ui.monitorChecks {
		if enabled {
			check.Enable()
		} else {
			check.Disable()
		}
	}
	for _, orientation := range ui.monitorOrientations {
		if enabled {
			orientation.Enable()
		} else {
			orientation.Disable()
		}
	}
}

func orientationDisplay(orientation config.WallpaperOrientation) string {
	return orientation.Label()
}

func orientationValue(display string) config.WallpaperOrientation {
	switch display {
	case config.WallpaperLandscapeOrientation.Label():
		return config.WallpaperLandscapeOrientation
	case config.WallpaperPortraitOrientation.Label():
		return config.WallpaperPortraitOrientation
	default:
		return config.WallpaperAnyOrientation
	}
}

func (ui *settingsUI) applyAutostart() {
	if ui.autostartCheck.Checked == ui.autostartOrigState {
		return
	}

	if !ui.autostartCheck.Checked {
		// Disabling now stops kPixiv immediately, not just future logins
		// (see internal/platform.DisableService). If this window is itself
		// running as the systemd-managed instance, confirming here will
		// close it. Confirm first so this isn't a surprise.
		dialog.ShowConfirm(
			"Stop KPixiv?",
			"This stops kPixiv now, not just at your next login.\n\n"+
				"Wallpaper rotation, fetching, and bookmark sync will stop until you turn this back on.",
			func(confirmed bool) {
				if !confirmed {
					ui.autostartCheck.SetChecked(ui.autostartOrigState)
					return
				}
				ui.doApplyAutostart()
			},
			ui.w,
		)
		return
	}

	ui.doApplyAutostart()
}

func (ui *settingsUI) doApplyAutostart() {
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
	ui.refreshMonitorStatus()
	if ui.ctrl.Config().Wallpaper.MultiMonitorEnabled {
		ui.currentWallpaperSection.Hide()
		ui.monitorWallpaperSection.Show()
	} else {
		ui.currentWallpaperSection.Show()
		ui.monitorWallpaperSection.Hide()
	}
	ui.refreshOverview()
	ui.refreshFirstRun()
	meta, err := ui.ctrl.CurrentWallpaper()
	if err != nil {
		ui.log.Error("Failed to get current wallpaper", "error", err)
	}

	if meta != nil && meta.ID != "" && meta.ID != ui.lastWallpaperID {
		ui.lastWallpaperID = meta.ID
		thumbPath := ui.ctrl.EnsureThumbnail(meta.ID)
		ui.statusThumbnail.File = thumbPath
		ui.statusThumbnail.Refresh()
	}

	if meta != nil && meta.ID != "" {
		ui.statusWallpaper.SetText(ui.formatWallpaperInfo(meta))
		ui.statusThumbnail.Show()
	} else {
		ui.statusWallpaper.SetText("No wallpaper set")
		ui.statusThumbnail.Hide()
	}

	ui.statusCached.SetText(ui.formatCachedCount())
	ui.statusLastRot.SetText(ui.formatLastChange())
	ui.statusNextRot.SetText(ui.formatNextChange())
	ui.statusLastFetch.SetText(ui.formatLastFetch())
	ui.statusNextFetch.SetText(ui.formatNextFetch())
	ui.statusLastBookmarkSync.SetText(ui.formatLastBookmarkSync())
	ui.statusNextBookmarkSync.SetText(ui.formatNextBookmarkSync())

	if ui.ctrl.Config().Bookmarks.Enabled {
		ui.bookmarkSyncRows.Show()
	} else {
		ui.bookmarkSyncRows.Hide()
	}
}

func (ui *settingsUI) refreshAutostartState() {
	enabled, err := ui.ctrl.ServiceEnabled()
	fyne.Do(func() {
		ui.autostartCheck.Enable()
		ui.autostartCheck.SetChecked(enabled && err == nil)
		ui.autostartOrigState = enabled && err == nil
		ui.autostartStatus.Hide()
	})
}

func (ui *settingsUI) formatWallpaperInfo(meta *storage.ImageMeta) string {
	title := meta.Title
	if title == "" {
		title = "Unknown artwork"
	}

	artist := meta.Artist
	if artist == "" {
		artist = "Unknown artist"
	}

	res := ""
	if meta.Width > 0 && meta.Height > 0 {
		res = strconv.Itoa(meta.Width) + " × " + strconv.Itoa(meta.Height)
	}

	src := formatSource(meta.Source, meta.Rank)
	return title + "\n" + artist + "\n" + res + "\n" + src
}

func (ui *settingsUI) formatCachedCount() string {
	return "Cached wallpapers: " + strconv.Itoa(ui.ctrl.CachedCount())
}

func (ui *settingsUI) formatLastChange() string {
	t := ui.ctrl.LastRotation()
	if t.IsZero() {
		return "Never"
	}

	return t.Format("Jan 02, 15:04")
}

func (ui *settingsUI) formatNextChange() string {
	lastRot := ui.ctrl.LastRotation()
	if lastRot.IsZero() {
		return "No rotation scheduled"
	}

	cfg := ui.ctrl.Config()
	interval := time.Duration(cfg.Wallpaper.SetInterval) * time.Minute
	return ui.formatCountdown(lastRot.Add(interval))
}

func (ui *settingsUI) formatLastFetch() string {
	t := ui.ctrl.LastFetch()
	if t.IsZero() {
		return "Never"
	}

	return t.Format("Jan 02, 15:04")
}

func (ui *settingsUI) formatNextFetch() string {
	cfg := ui.ctrl.Config()
	if !cfg.Wallpaper.FetchEnabled {
		return "Disabled"
	}

	last := ui.ctrl.LastFetch()
	if last.IsZero() {
		return "in " + formatMinutes(cfg.Wallpaper.FetchInterval)
	}

	return ui.formatCountdown(last.Add(time.Duration(cfg.Wallpaper.FetchInterval) * time.Minute))
}

func (ui *settingsUI) formatLastBookmarkSync() string {
	t := ui.ctrl.LastBookmarkSync()
	if t.IsZero() {
		return "Never"
	}

	return t.Format("Jan 02, 15:04")
}

func (ui *settingsUI) formatNextBookmarkSync() string {
	cfg := ui.ctrl.Config()
	if !cfg.Bookmarks.Enabled {
		return "Disabled"
	}

	last := ui.ctrl.LastBookmarkSync()
	if last.IsZero() {
		return "in " + formatMinutes(cfg.Bookmarks.SyncInterval)
	}

	return ui.formatCountdown(last.Add(time.Duration(cfg.Bookmarks.SyncInterval) * time.Minute))
}

// formatCountdown renders a countdown to an upcoming event, e.g., the next
// wallpaper change or fetch. The label column of the overview supplies the
// event name, so this only returns the pure data.
func (ui *settingsUI) formatCountdown(next time.Time) string {
	remaining := time.Until(next)
	if remaining <= 0 {
		return "Any moment now"
	}

	mins := int(remaining.Minutes())
	secs := int(remaining.Seconds()) % 60
	if mins > 0 {
		return fmt.Sprintf("in %dm %ds", mins, secs)
	}

	return fmt.Sprintf("in %ds", secs)
}

func formatMinutes(minutes int) string {
	return strconv.Itoa(minutes) + "m"
}

// rankingSubDisplay maps a ranking mode to its GUI select option.
func rankingSubDisplay(mode config.RankingMode) string {
	switch mode {
	case config.RankingWeeklyMode:
		return rankingSubWeekly
	case config.RankingMonthlyMode:
		return rankingSubMonthly
	default:
		return rankingSubDaily
	}
}

func feedSourceDisplay(cfg *config.Config) string {
	switch cfg.Wallpaper.QueueSource {
	case config.QueueSourceBookmarks:
		return feedSourceBookmarks
	case config.QueueSourceAll:
		return feedSourceAll
	default:
		switch cfg.Pixiv.Ranking {
		case config.RankingWeeklyMode:
			return feedSourceWeeklyRanking
		case config.RankingMonthlyMode:
			return feedSourceMonthlyRanking
		default:
			return feedSourceDailyRanking
		}
	}
}

func formatSource(source string, rank int) string {
	switch source {
	case "daily":
		if rank > 0 {
			return feedSourceDailyRanking + " (#" + strconv.Itoa(rank) + ")"
		}
		return feedSourceDailyRanking
	case "weekly":
		if rank > 0 {
			return feedSourceWeeklyRanking + " (#" + strconv.Itoa(rank) + ")"
		}
		return feedSourceWeeklyRanking
	case "monthly":
		if rank > 0 {
			return feedSourceMonthlyRanking + " (#" + strconv.Itoa(rank) + ")"
		}
		return feedSourceMonthlyRanking
	case "bookmarks":
		return "Bookmarks"
	default:
		if source != "" {
			return source
		}
		return "Unknown"
	}
}
