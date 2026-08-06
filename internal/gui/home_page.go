package gui

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/alphonse927/kpixiv/internal/human"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

func (ui *settingsUI) buildHomePage() fyne.CanvasObject {
	bold := fyne.TextStyle{Bold: true}
	section := func(title string) fyne.CanvasObject {
		l := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, bold)
		l.SizeName = theme.SizeNameSubHeadingText
		return l
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

	statusCol := container.NewVBox(
		subHeader("Status"),
		container.New(
			layout.NewFormLayout(),
			overviewLabel("Daemon"), statusValue(ui.statusDaemonDot, ui.statusDaemon),
			overviewLabel("Pixiv"), statusValue(ui.statusAuthDot, ui.statusAuth),
			overviewLabel("Cached"), ui.statusCache,
			overviewLabel("History"), ui.statusHistory,
		),
	)

	ui.bookmarkSyncRows = container.New(
		layout.NewFormLayout(),
		overviewLabel("Last sync"), ui.statusLastBookmarkSync,
		overviewLabel("Next sync"), ui.statusNextBookmarkSync,
	)

	timingMainRows := container.New(
		layout.NewFormLayout(),
		overviewLabel("Last change"), ui.statusLastRot,
		overviewLabel("Next change"), ui.statusNextRot,
		overviewLabel("Last fetch"), ui.statusLastFetch,
		overviewLabel("Next fetch"), ui.statusNextFetch,
	)

	timingCol := container.NewVBox(
		subHeader("Timing"),
		timingMainRows,
		ui.bookmarkSyncRows,
	)

	dashboard := container.NewVBox(
		section("Overview"),
		container.NewPadded(container.NewGridWithColumns(2, statusCol, timingCol)),
	)

	bg := canvas.NewRectangle(panelColor())
	bg.CornerRadius = theme.Size(theme.SizeNameCardRadius)
	nextWall := container.New(layout.NewStackLayout(),
		bg,
		container.NewPadded(container.NewVBox(
			widget.NewLabelWithStyle("Next wallpaper", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewPadded(ui.statusNextWall),
		)),
	)

	dashboard.Add(container.NewPadded(nextWall))

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

// statusValue lays a small colored status dot out in front of a status label,
// vertically aligned against the text.
func statusValue(dot *canvas.Circle, label *widget.Label) fyne.CanvasObject {
	const dotSize = float32(9)
	boxed := container.New(
		layout.NewGridWrapLayout(fyne.NewSize(dotSize, dotSize)),
		dot,
	)
	return container.NewHBox(container.NewCenter(boxed), label)
}

// setStatus updates a status label's text along with its importance and the
// matching indicator dot color, reusing the theme's semantic colors.
func setStatus(label *widget.Label, dot *canvas.Circle, text string, importance widget.Importance, fill color.Color) {
	label.SetText(text)
	label.Importance = importance
	label.Refresh()
	dot.FillColor = fill
	dot.Refresh()
}

// pluralEntry returns "entry" or "entries" depending on the count.
func pluralEntry(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}

// overviewLabel renders an overview row label. Row labels are the bottom level
// of the screen's type hierarchy: regular weight (not bold), right-aligned so
// each row's value column lines up.
func overviewLabel(text string) fyne.CanvasObject {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{})
}

// subHeader renders a subsection heading (e.g. "Status", "Timing") at the
// mid-level of the screen's type hierarchy: section headers are the largest
// (SubHeading, bold), subsection headers are bold at the Text size, and row
// labels are regular at the Text size.
func subHeader(text string) fyne.CanvasObject {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// panelColor returns a color slightly lighter than the theme background, so a
// contained block reads as a distinct panel in both light and dark themes.
func panelColor() color.Color {
	r, g, b, _ := theme.Color(theme.ColorNameBackground).RGBA()
	toChannel := func(v uint32) float64 { return float64(v / 255) }
	const lift = 0.10
	lighten := func(v float64) uint8 {
		return uint8(math.Round(v + (255-v)*lift))
	}
	return color.NRGBA{
		R: lighten(toChannel(r)),
		G: lighten(toChannel(g)),
		B: lighten(toChannel(b)),
		A: 255,
	}
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

		artworkTitle := meta.Title
		if artworkTitle == "" {
			artworkTitle = "Untitled artwork"
		}

		var secondary []string
		if meta.Artist != "" {
			secondary = append(secondary, meta.Artist)
		}
		if meta.Width > 0 && meta.Height > 0 {
			secondary = append(secondary, strconv.Itoa(meta.Width)+" × "+strconv.Itoa(meta.Height))
		}
		secondary = append(secondary, formatSource(meta.Source, meta.Rank))

		titleLine := widget.NewRichText(
			&widget.TextSegment{
				Text:  fmt.Sprintf("%s (%s)", name, id),
				Style: widget.RichTextStyle{SizeName: theme.SizeNameText, ColorName: theme.ColorNameForeground, TextStyle: fyne.TextStyle{Bold: true}},
			},
			&widget.TextSegment{
				Text:  artworkTitle,
				Style: widget.RichTextStyle{SizeName: theme.SizeNameText, ColorName: theme.ColorNameForeground, TextStyle: fyne.TextStyle{Bold: true}},
			},
			&widget.TextSegment{
				Text:  strings.Join(secondary, " · "),
				Style: widget.RichTextStyle{SizeName: theme.SizeNameText, ColorName: theme.ColorNamePlaceHolder},
			},
		)
		titleLine.Wrapping = fyne.TextWrapWord

		info := container.NewVBox(titleLine)

		ui.monitorStatus.Add(container.NewBorder(nil, nil, thumbnail, nil, info))
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
		setStatus(ui.statusDaemon, ui.statusDaemonDot, "Running", widget.SuccessImportance, theme.Color(theme.ColorNameSuccess))
	} else {
		setStatus(ui.statusDaemon, ui.statusDaemonDot, "Stopped", widget.WarningImportance, theme.Color(theme.ColorNameWarning))
	}

	if ui.ctrl.PixivLoggedIn() {
		user := ui.ctrl.PixivUserName()
		text := "Connected"
		if user != "" {
			text = "Connected (" + user + ")"
		}
		setStatus(ui.statusAuth, ui.statusAuthDot, text, widget.SuccessImportance, theme.Color(theme.ColorNameSuccess))
	} else {
		setStatus(ui.statusAuth, ui.statusAuthDot, "Not connected", widget.DangerImportance, theme.Color(theme.ColorNameError))
	}

	cacheText := fmt.Sprintf("%d image%s", stats.Count, human.Plural(stats.Count))
	if stats.Size > 0 {
		cacheText += " (" + human.Bytes(stats.Size) + ")"
	}
	ui.statusCache.SetText(cacheText)

	count := ui.ctrl.HistoryCount()
	ui.statusHistory.SetText(fmt.Sprintf("%d %s", count, pluralEntry(count)))

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

	var text strings.Builder
	for i, w := range warnings {
		if i > 0 {
			text.WriteString("\n")
		}
		text.WriteString("• " + w)
	}

	ui.statusWarnings.SetText(text.String())
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
