package gui

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/alphonse927/kpixiv/internal/human"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
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

	ui.currentWallpaperSection = container.NewPadded(container.NewVBox(section("Current Wallpaper"), wallpaperPanel))
	ui.monitorWallpaperSection = container.NewPadded(container.NewVBox(section("Monitor Wallpapers"), ui.monitorStatus))

	ui.firstRunTitle = widget.NewLabelWithStyle("Welcome to kPixiv", fyne.TextAlignLeading, bold)
	ui.firstRunDetail = widget.NewLabel("")
	ui.firstRunDetail.Wrapping = fyne.TextWrapWord
	ui.firstRunActions = container.NewHBox()

	fetchBtn := widget.NewButton("Fetch Wallpapers", func() {
		if err := ui.ctrl.FetchNow(); err != nil {
			ui.log.Warn("Failed to start fetch", "error", err)
		}
	})
	loginBtn := widget.NewButton("Set up Pixiv Login", func() {
		ui.selectPage(AccountPage)
	})
	ui.firstRunActions.Add(fetchBtn)
	ui.firstRunActions.Add(loginBtn)

	ui.firstRunBox = container.NewPadded(
		container.NewVBox(ui.firstRunTitle, ui.firstRunDetail, ui.firstRunActions),
	)

	dashboard := container.NewVBox(
		section("Overview"),
		dashboardRow("Daemon", ui.statusDaemon),
		dashboardRow("Pixiv", ui.statusAuth),
		dashboardRow("Cached", ui.statusCache),
		dashboardRow("History", ui.statusHistory),
		dashboardRow("Last change", ui.statusLastRot),
		dashboardRow("Next change", ui.statusNextRot),
		dashboardRow("Next wallpaper", ui.statusNextWall),
	)

	content := container.NewVBox(
		ui.firstRunBox,
		ui.currentWallpaperSection,
		ui.monitorWallpaperSection,
		container.NewPadded(dashboard),
		container.NewPadded(container.NewVBox(ui.statusWarnings)),
		container.NewPadded(
			container.NewVBox(
				section("System Integration"),
				ui.autostartCheck,
				ui.autostartStatus,
			),
		),
		container.NewPadded(widget.NewButton("View Logs...", ui.showLogViewer)),
	)

	return container.NewScroll(container.NewPadded(content))
}

// dashboardRow creates an aligned label pair used by the overview section.
func dashboardRow(key string, value *widget.Label) fyne.CanvasObject {
	label := widget.NewLabelWithStyle(key, fyne.TextAlignLeading, fyne.TextStyle{})
	return container.NewGridWithColumns(2, label, value)
}

func (ui *settingsUI) refreshMonitorStatus() {
	ui.monitorStatus.Objects = nil
	if !ui.ctrl.Config().Wallpaper.MultiMonitorEnabled {
		ui.monitorStatus.Add(widget.NewLabel("Enable multi-monitor mode in the Monitors tab."))
		ui.monitorStatus.Refresh()
		return
	}

	wallpapers, err := ui.ctrl.MonitorWallpapers()
	if err != nil || len(wallpapers) == 0 {
		ui.monitorStatus.Add(widget.NewLabel("No active monitor wallpapers found."))
		ui.monitorStatus.Refresh()
		return
	}

	screens := make(map[string]wallpaper.Screen)
	if screenList, screensErr := ui.ctrl.Monitors(); screensErr == nil {
		for _, screen := range screenList {
			screens[screen.ID] = screen
		}
	}

	ids := make([]string, 0, len(wallpapers))
	for id := range wallpapers {
		ids = append(ids, id)
	}

	sort.Strings(ids)
	for _, id := range ids {
		meta := wallpapers[id]
		if meta == nil {
			continue
		}
		thumbnail := canvas.NewImageFromFile(ui.ctrl.EnsureThumbnail(meta.ID))
		thumbnail.FillMode = canvas.ImageFillContain
		thumbnail.SetMinSize(fyne.NewSize(110, 80))
		screen, ok := screens[id]
		if !ok {
			screen = wallpaper.Screen{ID: id}
		}

		name := screen.Label()
		ui.monitorStatus.Add(container.NewBorder(nil, nil, thumbnail, nil,
			widget.NewLabel(fmt.Sprintf("%s (%s)\n%s", name, id, ui.formatWallpaperInfo(meta)))))
	}

	ui.monitorStatus.Refresh()
}

func (ui *settingsUI) refreshOverview() {
	stats, err := ui.ctrl.CacheStats()
	if err != nil {
		ui.log.Warn("Failed to load cache statistics", "error", err)
		stats = storage.CacheStats{}
	}

	if ui.ctrl.SchedulerRunning() {
		ui.statusDaemon.SetText("Running")
	} else {
		ui.statusDaemon.SetText("Stopped")
	}

	if ui.ctrl.PixivLoggedIn() {
		user := ui.ctrl.PixivUserName()
		if user != "" {
			ui.statusAuth.SetText("Connected (" + user + ")")
		} else {
			ui.statusAuth.SetText("Connected")
		}
	} else {
		ui.statusAuth.SetText("Not connected")
	}

	cacheText := fmt.Sprintf("%d image%s", stats.Count, human.Plural(stats.Count))
	if stats.Size > 0 {
		cacheText += " (" + human.Bytes(stats.Size) + ")"
	}
	ui.statusCache.SetText(cacheText)

	ui.statusHistory.SetText(fmt.Sprintf("%d entr%s", ui.ctrl.HistoryCount(), human.Plural(ui.ctrl.HistoryCount())))

	ui.statusNextWall.SetText(ui.formatNextWallpaper())

	ui.refreshWarnings(stats.Count)
}

func (ui *settingsUI) refreshWarnings(cached int) {
	var warnings []string
	if cached == 0 {
		warnings = append(warnings, "No wallpapers downloaded yet. Use \"Fetch Wallpapers\" to get started.")
	}
	if !ui.ctrl.PixivLoggedIn() {
		warnings = append(warnings, "Not connected to Pixiv. Log in to enable bookmarks and sync.")
	}
	if ui.ctrl.SchedulerRunning() && ui.ctrl.Config().Wallpaper.RotationEnabled && cached == 0 {
		warnings = append(warnings, "Rotation is enabled but there is nothing to rotate yet.")
	}

	if len(warnings) == 0 {
		ui.statusWarnings.Hide()
		ui.statusWarnings.SetText("")
		return
	}

	text := ""
	for i, w := range warnings {
		if i > 0 {
			text += "\n"
		}
		text += "• " + w
	}
	ui.statusWarnings.SetText(text)
	ui.statusWarnings.Show()
}

func (ui *settingsUI) refreshFirstRun() {
	cached := ui.ctrl.CachedCount()
	if cached > 0 {
		ui.firstRunBox.Hide()
		return
	}

	ui.firstRunTitle.SetText("Welcome to kPixiv")
	if ui.ctrl.PixivLoggedIn() {
		ui.firstRunDetail.SetText("There are no wallpapers yet. Fetch the daily ranking to fill your wallpaper rotation.")
	} else {
		ui.firstRunDetail.SetText("There are no wallpapers yet. Connect your Pixiv account and fetch the daily ranking to get started.")
	}

	ui.firstRunActions.Objects = nil
	fetchBtn := widget.NewButton("Fetch Wallpapers", func() {
		if err := ui.ctrl.FetchNow(); err != nil {
			ui.log.Warn("Failed to start fetch", "error", err)
		}
	})
	ui.firstRunActions.Add(fetchBtn)
	if !ui.ctrl.PixivLoggedIn() {
		loginBtn := widget.NewButton("Set up Pixiv Login", func() {
			ui.selectPage(AccountPage)
		})
		ui.firstRunActions.Add(loginBtn)
	}
	ui.firstRunActions.Refresh()
	ui.firstRunBox.Show()
}

func (ui *settingsUI) formatNextWallpaper() string {
	if ui.ctrl.Config().Wallpaper.MultiMonitorEnabled {
		return ui.formatMonitorNextWallpapers()
	}

	id := ui.ctrl.NextWallpaperID()
	if id == "" {
		return "Queue is empty"
	}

	meta, err := ui.ctrl.WallpaperMeta(id)
	if err != nil || meta == nil {
		return id
	}

	return ui.artworkLabel(id, meta)
}

func (ui *settingsUI) formatMonitorNextWallpapers() string {
	next := ui.ctrl.MonitorNextWallpapers()
	if len(next) == 0 {
		return "Per-monitor queues are empty"
	}

	names := make(map[string]string)
	if screens, err := ui.ctrl.Monitors(); err == nil {
		for _, screen := range screens {
			names[screen.ID] = screen.Name
		}
	}

	ids := make([]string, 0, len(next))
	for id := range next {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		name := names[id]
		if name == "" {
			name = "Screen " + id
		}
		lines = append(lines, name+": "+ui.artworkLabel(next[id], nil))
	}
	return strings.Join(lines, "\n")
}

// artworkLabel formats an artwork ID plus its title/artist when known.
func (ui *settingsUI) artworkLabel(id string, meta *storage.ImageMeta) string {
	if meta == nil {
		var err error
		if meta, err = ui.ctrl.WallpaperMeta(id); err != nil || meta == nil {
			return id
		}
	}

	label := id
	if meta.Title != "" {
		label += " " + meta.Title
	}
	if meta.Artist != "" {
		label += " by " + meta.Artist
	}
	return label
}
