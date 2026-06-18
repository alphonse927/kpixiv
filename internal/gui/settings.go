package gui

import (
	"image/color"
	"log/slog"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/alphonse927/kpixiv/internal/assets"
	"github.com/alphonse927/kpixiv/internal/config"
)

// numericalEntry only accepts digits and rejects non-numeric paste.
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

// tintedBG tints the background while preserving the system light/dark variant
// for all other elements (inputs, text, buttons).
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
	w       fyne.Window
	log     *slog.Logger
	onApply OnApply

	cfg           *config.Config
	downloadPath  *widget.Entry
	setInterval   *numericalEntry
	fetchInterval *numericalEntry
	historyLimit  *numericalEntry
	cleanupDays   *numericalEntry
	ranking       *widget.Select
	minWidth      *numericalEntry
	minHeight     *numericalEntry
	lockScreen    *widget.Check
}

func newSettingsUI(a fyne.App, cfg *config.Config, log *slog.Logger, onApply OnApply) *settingsUI {
	ui := &settingsUI{
		log:     log,
		onApply: onApply,
		cfg:     cfg,
	}
	ui.build(a)
	return ui
}

func (ui *settingsUI) build(a fyne.App) {
	w := a.NewWindow("kPixiv – Settings")
	w.Resize(fyne.NewSize(560, 600))
	w.CenterOnScreen()
	w.SetIcon(fyne.NewStaticResource("kpixiv", assets.IconPNG))
	ui.w = w
	w.SetCloseIntercept(ui.hide)

	ui.createWidgets()
	w.SetContent(ui.buildLayout())
}

func (ui *settingsUI) createWidgets() {
	ui.downloadPath = widget.NewEntry()
	ui.downloadPath.SetText(ui.cfg.DownloadPath)
	ui.downloadPath.PlaceHolder = "~/Pictures/KPixiv"

	ui.setInterval = newNumericalEntry()
	ui.setInterval.SetText(strconv.Itoa(ui.cfg.Wallpaper.SetInterval))

	ui.fetchInterval = newNumericalEntry()
	ui.fetchInterval.SetText(strconv.Itoa(ui.cfg.Wallpaper.FetchInterval))

	ui.historyLimit = newNumericalEntry()
	ui.historyLimit.SetText(strconv.Itoa(ui.cfg.Wallpaper.HistoryLimit))

	ui.cleanupDays = newNumericalEntry()
	ui.cleanupDays.SetText(strconv.Itoa(ui.cfg.Wallpaper.CleanupDays))

	ui.ranking = widget.NewSelect([]string{"daily", "weekly", "monthly"}, nil)
	ui.ranking.SetSelected(ui.cfg.Pixiv.Ranking.String())

	ui.minWidth = newNumericalEntry()
	ui.minWidth.SetText(strconv.Itoa(ui.cfg.Pixiv.MinWidth))

	ui.minHeight = newNumericalEntry()
	ui.minHeight.SetText(strconv.Itoa(ui.cfg.Pixiv.MinHeight))

	ui.lockScreen = widget.NewCheck("Set Lock Screen", nil)
	ui.lockScreen.SetChecked(ui.cfg.KDE.SetLockScreen)
}

//nolint:funlen // widget construction and layout is inherently verbose
func (ui *settingsUI) buildLayout() fyne.CanvasObject {
	w := ui.w

	browse := widget.NewButton("Browse...", func() {
		dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				ui.downloadPath.SetText(uri.Path())
			}
		}, w).Show()
	})

	apply := func() {
		cfg := ui.cfg

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
		cfg.Validate()

		if err := config.Save(cfg.ConfigPath, cfg); err != nil {
			dialog.ShowError(err, w)
			return
		}

		ui.log.Info("Settings applied")
		if ui.onApply != nil {
			ui.onApply()
		}
	}

	bold := fyne.TextStyle{Bold: true}
	section := func(title string) fyne.CanvasObject {
		return widget.NewLabelWithStyle(title, fyne.TextAlignLeading, bold)
	}

	desc := func(text string) fyne.CanvasObject {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{})
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

	intervals := sideBySide(
		container.NewVBox(
			field("Set Interval (min)", ui.setInterval),
			desc("How often to switch wallpapers"),
		),
		container.NewVBox(
			field("Fetch Interval (min)", ui.fetchInterval),
			desc("How often to download new wallpapers"),
		),
	)

	dims := sideBySide(
		field("Min Width", ui.minWidth),
		field("Min Height", ui.minHeight),
	)

	other := sideBySide(
		container.NewVBox(
			field("History Limit", ui.historyLimit),
			desc("How many wallpapers to keep in rotation history"),
		),
		container.NewVBox(
			field("Cleanup Days", ui.cleanupDays),
			desc("Remove downloaded images older than this"),
		),
	)

	buttons := container.NewHBox(
		layout.NewSpacer(),
		widget.NewButton("Cancel", ui.hide),
		widget.NewButton("Apply", apply),
		widget.NewButton("Save & Close", func() {
			apply()
			ui.log.Info("Settings saved and closed")
			w.Hide()
		}),
	)

	return container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				section("Intervals"),
				intervals,

				widget.NewSeparator(),
				section("Ranking"),
				desc("Which Pixiv ranking feed to pull wallpapers from"),
				ui.ranking,

				widget.NewSeparator(),
				section("Dimensions"),
				desc("Minimum image size to download (smaller images are filtered out)"),
				dims,

				widget.NewSeparator(),
				section("Storage"),
				desc("Download Directory"),
				container.NewBorder(nil, nil, nil, browse, ui.downloadPath),
				other,

				widget.NewSeparator(),
				ui.lockScreen,
				desc("Also apply the current wallpaper to the KDE lock screen"),
			),
		),
		container.NewPadded(buttons),
	)
}

func (ui *settingsUI) show() {
	ui.w.Show()
}

func (ui *settingsUI) hide() {
	ui.log.Debug("Settings window closed")
	ui.w.Hide()
}

func (ui *settingsUI) update(cfg *config.Config) {
	ui.cfg = cfg
	ui.downloadPath.SetText(cfg.DownloadPath)
	ui.setInterval.SetText(strconv.Itoa(cfg.Wallpaper.SetInterval))
	ui.fetchInterval.SetText(strconv.Itoa(cfg.Wallpaper.FetchInterval))
	ui.historyLimit.SetText(strconv.Itoa(cfg.Wallpaper.HistoryLimit))
	ui.cleanupDays.SetText(strconv.Itoa(cfg.Wallpaper.CleanupDays))
	ui.ranking.SetSelected(cfg.Pixiv.Ranking.String())
	ui.minWidth.SetText(strconv.Itoa(cfg.Pixiv.MinWidth))
	ui.minHeight.SetText(strconv.Itoa(cfg.Pixiv.MinHeight))
	ui.lockScreen.SetChecked(cfg.KDE.SetLockScreen)
}
