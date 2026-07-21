package gui

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
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
	content := container.NewVBox(
		ui.currentWallpaperSection,
		ui.monitorWallpaperSection,
		container.NewPadded(
			container.NewVBox(
				section("Quick Statistics"),
				ui.statusCached,
				ui.statusNextRot,
				ui.statusLastRot,
			),
		),
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
	names := make(map[string]string)
	models := make(map[string]string)
	if screens, screensErr := ui.ctrl.Monitors(); screensErr == nil {
		for _, screen := range screens {
			names[screen.ID] = screen.Name
			models[screen.ID] = screen.Model
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
		thumbnail := canvas.NewImageFromFile(ui.ctrl.ThumbnailPath(meta.ID))
		thumbnail.FillMode = canvas.ImageFillContain
		thumbnail.SetMinSize(fyne.NewSize(110, 80))
		name := names[id]
		if name == "" {
			name = "Screen " + id
		}
		if m := models[id]; m != "" {
			name = name + " (" + m + ")"
		}
		ui.monitorStatus.Add(container.NewBorder(nil, nil, thumbnail, nil,
			widget.NewLabel(fmt.Sprintf("%s (%s)\n%s", name, id, ui.formatWallpaperInfo(meta)))))
	}
	ui.monitorStatus.Refresh()
}
