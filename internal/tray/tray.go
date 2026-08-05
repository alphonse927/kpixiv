package tray

import (
	"context"
	"log/slog"
	"time"

	"fyne.io/systray"
	"github.com/alphonse927/kpixiv/internal/assets"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

// monitorSlotCount is how many monitor submenus are pre-allocated at startup.
// Screens beyond this capacity are ignored; 8 covers realistic multi-display
// setups while keeping the pre-allocated menu small.
const monitorSlotCount = 8

// Actions a monitor submenu can dispatch. They are forwarded over a single
// channel, so the event loop keeps a static select case list.
const (
	actionNext      = "next"
	actionFavorite  = "favorite"
	actionOpenFile  = "open-file"
	actionOpenPixiv = "open-pixiv"
	actionBookmark  = "bookmark"
	actionExclude   = "exclude"
)

type Controller interface {
	Start() error
	NextWallpaper() error
	PauseRotation()
	ResumeRotation()
	PixivLoggedIn() bool
	PixivUserName() string
	LogoutFromPixiv() error
	CopyCurrentArtwork() error
	BookmarkCurrentArtwork() error
	IsWallpaperBookmarked(artworkID string) bool
	CurrentWallpaperID() string
	OpenCurrentArtwork() error
	OpenCurrentArtworkInPixiv() error
	ExcludeCurrentWallpaper() error

	MultiMonitorEnabled() bool
	Monitors() ([]wallpaper.Screen, error)
	MonitorWallpapers() (map[string]*storage.ImageMeta, error)
	NextWallpaperForMonitor(monitorID string) error
	NextWallpaperForAllMonitors() error
	BookmarkWallpaper(artworkID string) error
	ExcludeWallpaper(artworkID string) error
	OpenWallpaperFile(artworkID string) error
	OpenWallpaperInPixiv(artworkID string) error
	CopyWallpaperToFavorites(artworkID string) error

	ShowSettingsWindow() error
	ShowAccountSettings() error
	Shutdown()
}

// monitorAction is a fanned-in event describing which slot and action a click
// corresponds to. Slot contents are looked up at dispatch time from the
// trayMenu's mutable state, so stale closures are not relied upon across
// rebuilds.
type monitorAction struct {
	slot   int
	action string
}

// monitorSlot is one pre-allocated per-monitor submenu. Menu items are created
// once at startup and reused (relabeled/showed/hidden) across rebuilds; only
// the mutable screenID/artworkID fields change.
type monitorSlot struct {
	submenu   *systray.MenuItem
	header    *systray.MenuItem
	next      *systray.MenuItem
	favorite  *systray.MenuItem
	openFile  *systray.MenuItem
	openPixiv *systray.MenuItem
	bookmark  *systray.MenuItem
	exclude   *systray.MenuItem

	screenID  string
	artworkID string
}

type trayMenu struct {
	ctrl Controller
	log  *slog.Logger

	next            *systray.MenuItem
	rotate          *systray.MenuItem
	login           *systray.MenuItem
	bookmarkCurrent *systray.MenuItem
	logout          *systray.MenuItem
	favoriteCurrent *systray.MenuItem
	openCurrent     *systray.MenuItem
	openPixiv       *systray.MenuItem
	excludeCurrent  *systray.MenuItem
	settings        *systray.MenuItem
	quit            *systray.MenuItem

	monitorSlots   []*monitorSlot
	monitorActions chan monitorAction

	// fallbackFlat is set when multi-monitor mode is active, but the monitor
	// list/wallpapers could not be resolved, so the flat single-monitor menu is
	// shown instead. It is recomputed on every rebuild.
	fallbackFlat bool
	// itemsCreated guards one-time creation of the static and slot menu items.
	itemsCreated bool

	rebuildCh chan struct{}
}

// Run starts the tray event loop and wires it to the application controller.
func Run(appCtx context.Context, controller Controller) {
	systray.Run(func() { onReady(appCtx, controller) }, func() {
		controller.Shutdown()
	})
}

func onReady(appCtx context.Context, controller Controller) {
	tm := &trayMenu{
		ctrl:           controller,
		log:            logger.WithComponent("tray"),
		monitorActions: make(chan monitorAction),
		rebuildCh:      make(chan struct{}, 1),
	}

	systray.SetTitle("kPixiv")
	systray.SetTooltip("kPixiv Wallpaper Manager")

	if icon := assets.IconPNG; len(icon) > 0 {
		systray.SetIcon(icon)
	}

	if err := controller.Start(); err != nil {
		tm.log.Error("Failed to start application controller", "error", err)
	}

	tm.buildMenu()

	// Register as rebuild target (socket-ready foundation).
	if r, ok := controller.(interface{ SetTrayRebuilder(func()) }); ok {
		r.SetTrayRebuilder(tm.RebuildMenu)
	}

	go tm.eventLoop(appCtx)
}

// buildMenu constructs the tray menu (once) and repopulates the per-monitor
// submenus from the current controller state. It is called at startup and
// again whenever RebuildMenu fires, so monitor count or multi-monitor mode
// changes take effect.
func (tm *trayMenu) buildMenu() {
	tm.createItemsOnce()

	multi := tm.ctrl.MultiMonitorEnabled()
	tm.fallbackFlat = false
	if multi {
		tm.fallbackFlat = !tm.populateMonitorSlots()
	} else {
		// Ensure any previously shown monitor submenus are hidden again when
		// returning to single-monitor mode.
		tm.hideAllSlots()
	}

	tm.applyFlatActionVisibility(multi && !tm.fallbackFlat)
	tm.updateAuthItems()
}

// createItemsOnce allocates every menu item, including the fixed number of
// pre-created monitor submenus (hidden by default). Static items keep their
// ClickedCh channels across rebuilds so the event loop select stays valid.
func (tm *trayMenu) createItemsOnce() {
	if tm.itemsCreated {
		return
	}
	tm.itemsCreated = true

	tm.next = systray.AddMenuItem("Next Wallpaper", "Immediately switch wallpaper")
	tm.rotate = systray.AddMenuItemCheckbox("Rotate Wallpaper", "Enable or pause wallpaper rotation", true)

	systray.AddSeparator()

	tm.login = systray.AddMenuItem("Login to Pixiv", "Connect your Pixiv account")
	tm.bookmarkCurrent = systray.AddMenuItem("Bookmark Current Artwork", "Bookmark the current artwork in Pixiv")
	tm.logout = systray.AddMenuItem("Logout from Pixiv", "Forget the saved Pixiv session")

	systray.AddSeparator()

	tm.favoriteCurrent = systray.AddMenuItem("Copy to Favorites", "Copy the current wallpaper to the favorite directory")
	tm.openCurrent = systray.AddMenuItem("Open Current Artwork", "Open currently active image")
	tm.openPixiv = systray.AddMenuItem("Open Artwork in Pixiv", "Open the current artwork's Pixiv page in your browser")
	tm.excludeCurrent = systray.AddMenuItem("Exclude Current Wallpaper", "Blacklist the current wallpaper and switch away")

	tm.monitorSlots = make([]*monitorSlot, monitorSlotCount)
	for i := range tm.monitorSlots {
		slot := &monitorSlot{
			submenu: systray.AddMenuItem("", ""),
		}
		slot.submenu.Hide()
		slot.header = slot.submenu.AddSubMenuItem("", "")
		slot.header.Disable()
		slot.next = slot.submenu.AddSubMenuItem("Next Wallpaper", "Switch this monitor's wallpaper")
		slot.favorite = slot.submenu.AddSubMenuItem("Copy to Favorites", "Copy this monitor's wallpaper to the favorite directory")
		slot.openFile = slot.submenu.AddSubMenuItem("Open Artwork File", "Open this monitor's wallpaper file")
		slot.openPixiv = slot.submenu.AddSubMenuItem("Open Artwork in Pixiv", "Open this monitor's artwork in Pixiv")
		slot.bookmark = slot.submenu.AddSubMenuItem("Bookmark Artwork", "Bookmark this monitor's artwork in Pixiv")
		slot.exclude = slot.submenu.AddSubMenuItem("Exclude Wallpaper", "Blacklist this monitor's wallpaper and switch away")
		tm.monitorSlots[i] = slot
		tm.forwardSlot(slot, i)
	}

	systray.AddSeparator()
	tm.settings = systray.AddMenuItem("Settings", "Open settings window")
	tm.quit = systray.AddMenuItem("Quit", "Quit kPixiv")
}

// populateMonitorSlots fills the pre-allocated monitor submenus from the
// controller's monitor state. It returns true on success. On error the slots
// are hidden, and false is returned, so buildMenu falls back to the flat menu.
func (tm *trayMenu) populateMonitorSlots() bool {
	screens, err := tm.ctrl.Monitors()
	if err != nil {
		tm.log.Warn("Failed to list monitors, falling back to flat menu", "error", err)
		tm.hideAllSlots()
		return false
	}

	wallpapers, err := tm.ctrl.MonitorWallpapers()
	if err != nil {
		tm.log.Warn("Failed to load monitor wallpapers, falling back to flat menu", "error", err)
		tm.hideAllSlots()
		return false
	}

	slots := buildSlotData(screens, wallpapers)

	shown := len(slots)
	if shown > monitorSlotCount {
		tm.log.Warn("More live monitors than tray slots, truncating", "count", len(slots))
		shown = monitorSlotCount
	}

	for i := range shown {
		slot := tm.monitorSlots[i]
		slot.screenID = slots[i].screen.ID
		slot.artworkID = ""
		if slots[i].meta != nil {
			slot.artworkID = slots[i].meta.ID
		}
		slot.submenu.SetTitle(screenSubmenuTitle(slots[i].screen))
		slot.header.SetTitle(slotHeaderTitle(slots[i].meta))
		slot.submenu.Show()
		tm.populateSlot(slot, slots[i].meta)
	}

	for i := shown; i < monitorSlotCount; i++ {
		tm.monitorSlots[i].submenu.Hide()
		tm.monitorSlots[i].screenID = ""
		tm.monitorSlots[i].artworkID = ""
	}

	return true
}

func (tm *trayMenu) hideAllSlots() {
	for _, slot := range tm.monitorSlots {
		slot.submenu.Hide()
		slot.screenID = ""
		slot.artworkID = ""
	}
}

// populateSlot enables or disables a slot's action items based on whether the
// screen has an assigned wallpaper.
func (tm *trayMenu) populateSlot(slot *monitorSlot, meta *storage.ImageMeta) {
	if meta == nil {
		slot.favorite.Disable()
		slot.openFile.Disable()
		slot.openPixiv.Disable()
		slot.exclude.Disable()
		slot.bookmark.Disable()
		slot.bookmark.SetTitle("Bookmark Artwork")
		return
	}

	slot.favorite.Enable()
	slot.openFile.Enable()
	slot.openPixiv.Enable()
	slot.exclude.Enable()
}

// buildSlotData maps each active screen to its assigned wallpaper metadata so
// the menu construction can be tested independently of the systray backend.
// A screen without an assigned wallpaper keeps a nil meta.
type slotData struct {
	screen wallpaper.Screen
	meta   *storage.ImageMeta
}

func buildSlotData(screens []wallpaper.Screen, wallpapers map[string]*storage.ImageMeta) []slotData {
	result := make([]slotData, 0, len(screens))
	for _, screen := range screens {
		result = append(result, slotData{screen: screen, meta: wallpapers[screen.ID]})
	}
	return result
}

// slotHeaderTitle is the artwork identification shown at the top of a monitor
// submenu. meta may be nil when no wallpaper is assigned yet.
func slotHeaderTitle(meta *storage.ImageMeta) string {
	if meta == nil {
		return "No wallpaper set"
	}

	title := meta.Title
	if title == "" {
		title = meta.ID
	}
	if meta.Artist != "" {
		title = title + " by " + meta.Artist
	}
	return title
}

// screenSubmenuTitle is the top-level submenu title for a screen, e.g. "DP-2 (Primary)".
func screenSubmenuTitle(screen wallpaper.Screen) string {
	title := screen.Label()
	if screen.Primary {
		title += " (Primary)"
	}
	return title
}

// applyFlatActionVisibility shows or hides the single-monitor "current
// wallpaper" action block and re-targets the top-level Next Wallpaper item.
func (tm *trayMenu) applyFlatActionVisibility(multi bool) {
	if multi {
		tm.bookmarkCurrent.Hide()
		tm.favoriteCurrent.Hide()
		tm.openCurrent.Hide()
		tm.openPixiv.Hide()
		tm.excludeCurrent.Hide()
		tm.next.SetTitle("Next Wallpaper (All Monitors)")
		tm.next.SetTooltip("Rotate the wallpaper on every monitor")
		return
	}

	tm.bookmarkCurrent.Show()
	tm.favoriteCurrent.Show()
	tm.openCurrent.Show()
	tm.openPixiv.Show()
	tm.excludeCurrent.Show()
	tm.next.SetTitle("Next Wallpaper")
	tm.next.SetTooltip("Immediately switch wallpaper")
}

//nolint:cyclop
func (tm *trayMenu) eventLoop(appCtx context.Context) {
	bookmarkTicker := time.NewTicker(3 * time.Second)
	defer bookmarkTicker.Stop()

	for {
		select {
		case <-tm.next.ClickedCh:
			var err error
			if tm.multiMonitorActive() {
				err = tm.ctrl.NextWallpaperForAllMonitors()
			} else {
				err = tm.ctrl.NextWallpaper()
			}
			if err != nil {
				tm.log.Warn("Failed to set next wallpaper", "error", err)
			}
			tm.buildMenu()

		case <-tm.rotate.ClickedCh:
			if tm.rotate.Checked() {
				tm.rotate.Uncheck()
				tm.ctrl.PauseRotation()
			} else {
				tm.rotate.Check()
				tm.ctrl.ResumeRotation()
			}

		case <-tm.login.ClickedCh:
			if err := tm.ctrl.ShowAccountSettings(); err != nil {
				tm.log.Warn("Failed to open account settings", "error", err)
			}

		case <-tm.logout.ClickedCh:
			if err := tm.ctrl.LogoutFromPixiv(); err != nil {
				tm.log.Warn("Failed to log out from Pixiv", "error", err)
			}
			tm.updateAuthItems()

		case <-tm.favoriteCurrent.ClickedCh:
			if err := tm.ctrl.CopyCurrentArtwork(); err != nil {
				tm.log.Warn("Failed to copy current artwork", "error", err)
			}

		case <-tm.bookmarkCurrent.ClickedCh:
			if err := tm.ctrl.BookmarkCurrentArtwork(); err != nil {
				tm.log.Warn("Failed to bookmark current artwork", "error", err)
			}
			tm.updateBookmarkItem()

		case <-tm.openCurrent.ClickedCh:
			if err := tm.ctrl.OpenCurrentArtwork(); err != nil {
				tm.log.Warn("Failed to open current artwork", "error", err)
			}

		case <-tm.openPixiv.ClickedCh:
			if err := tm.ctrl.OpenCurrentArtworkInPixiv(); err != nil {
				tm.log.Warn("Failed to open current artwork in Pixiv", "error", err)
			}

		case <-tm.excludeCurrent.ClickedCh:
			if err := tm.ctrl.ExcludeCurrentWallpaper(); err != nil {
				tm.log.Warn("Failed to exclude current artwork", "error", err)
			}
			tm.updateBookmarkItem()
			tm.updateMonitorSlots()

		case ev := <-tm.monitorActions:
			tm.handleMonitorAction(ev)

		case <-tm.settings.ClickedCh:
			if err := tm.ctrl.ShowSettingsWindow(); err != nil {
				tm.log.Warn("Failed to open settings", "error", err)
			}

		case <-tm.quit.ClickedCh:
			tm.ctrl.Shutdown()
			systray.Quit()
			return

		case <-bookmarkTicker.C:
			tm.updateBookmarkItem()
			tm.updateMonitorSlots()

		case <-tm.rebuildCh:
			tm.buildMenu()

		case <-appCtx.Done():
			systray.Quit()
			return
		}
	}
}

// multiMonitorActive reports whether the per-monitor submenus are actually
// shown (multi-monitor mode enabled and monitor enumeration succeeded).
func (tm *trayMenu) multiMonitorActive() bool {
	return tm.ctrl.MultiMonitorEnabled() && !tm.fallbackFlat
}

// handleMonitorAction dispatches a fanned per-monitor event. The slot's
// current screen and artwork ID are read from mutable state at dispatch time.
//
//nolint:cyclop
func (tm *trayMenu) handleMonitorAction(ev monitorAction) {
	if ev.slot < 0 || ev.slot >= len(tm.monitorSlots) {
		tm.log.Warn("Ignoring tray action for unknown monitor slot", "slot", ev.slot)
		return
	}

	slot := tm.monitorSlots[ev.slot]
	if slot.screenID == "" {
		tm.log.Warn("Ignoring tray action for inactive monitor slot", "slot", ev.slot)
		return
	}

	switch ev.action {
	case actionNext:
		if err := tm.ctrl.NextWallpaperForMonitor(slot.screenID); err != nil {
			tm.log.Warn("Failed to set next wallpaper", "screen", slot.screenID, "error", err)
		}
		// Rotation swaps this monitor's artwork, so repopulate its header.
		tm.buildMenu()

	case actionFavorite:
		if err := tm.ctrl.CopyWallpaperToFavorites(slot.artworkID); err != nil {
			tm.log.Warn("Failed to copy wallpaper to favorites", "screen", slot.screenID, "error", err)
		}

	case actionOpenFile:
		if err := tm.ctrl.OpenWallpaperFile(slot.artworkID); err != nil {
			tm.log.Warn("Failed to open wallpaper file", "screen", slot.screenID, "error", err)
		}

	case actionOpenPixiv:
		if err := tm.ctrl.OpenWallpaperInPixiv(slot.artworkID); err != nil {
			tm.log.Warn("Failed to open wallpaper in Pixiv", "screen", slot.screenID, "error", err)
		}

	case actionBookmark:
		if err := tm.ctrl.BookmarkWallpaper(slot.artworkID); err != nil {
			tm.log.Warn("Failed to bookmark wallpaper", "screen", slot.screenID, "error", err)
		}
		tm.updateMonitorSlots()

	case actionExclude:
		if err := tm.ctrl.ExcludeWallpaper(slot.artworkID); err != nil {
			tm.log.Warn("Failed to exclude wallpaper", "screen", slot.screenID, "error", err)
		}
		// Exclusion switches this monitor to its next wallpaper, so repopulate.
		tm.buildMenu()
	}
}

// forwardSlot spawns a goroutine that mirrors every click on a monitor slot's
// action items into the shared monitorActions channel.
func (tm *trayMenu) forwardSlot(slot *monitorSlot, index int) {
	go func() {
		for {
			select {
			case <-slot.next.ClickedCh:
				tm.monitorActions <- monitorAction{slot: index, action: actionNext}
			case <-slot.favorite.ClickedCh:
				tm.monitorActions <- monitorAction{slot: index, action: actionFavorite}
			case <-slot.openFile.ClickedCh:
				tm.monitorActions <- monitorAction{slot: index, action: actionOpenFile}
			case <-slot.openPixiv.ClickedCh:
				tm.monitorActions <- monitorAction{slot: index, action: actionOpenPixiv}
			case <-slot.bookmark.ClickedCh:
				tm.monitorActions <- monitorAction{slot: index, action: actionBookmark}
			case <-slot.exclude.ClickedCh:
				tm.monitorActions <- monitorAction{slot: index, action: actionExclude}
			}
		}
	}()
}

// RebuildMenu is called by the controller when config changes (e.g., the
// multi-monitor toggle or auth state), triggering a full repopulation of the
// menu on the event loop.
func (tm *trayMenu) RebuildMenu() {
	select {
	case tm.rebuildCh <- struct{}{}:
	default:
	}
}

func (tm *trayMenu) updateAuthItems() {
	if tm.ctrl.PixivLoggedIn() {
		userName := tm.ctrl.PixivUserName()
		if userName != "" {
			tm.login.SetTitle("Pixiv: " + userName)
		} else {
			tm.login.SetTitle("Pixiv Connected")
		}
		tm.login.Disable()
		tm.logout.Show()
		tm.logout.Enable()
		if !tm.multiMonitorActive() {
			tm.bookmarkCurrent.Enable()
			tm.updateBookmarkItem()
		}
		tm.updateMonitorSlots()
		return
	}

	tm.login.SetTitle("Login to Pixiv")
	tm.login.Enable()
	tm.logout.Hide()
	tm.logout.Disable()
	if !tm.multiMonitorActive() {
		tm.bookmarkCurrent.Disable()
		tm.bookmarkCurrent.SetTitle("Bookmark Current Artwork")
	}
	tm.updateMonitorSlots()
}

func (tm *trayMenu) updateBookmarkItem() {
	if tm.ctrl.IsWallpaperBookmarked(tm.ctrl.CurrentWallpaperID()) {
		tm.bookmarkCurrent.SetTitle("Bookmarked")
		tm.bookmarkCurrent.Disable()
	} else {
		tm.bookmarkCurrent.SetTitle("Bookmark Current Artwork")
		tm.bookmarkCurrent.Enable()
	}
}

// updateMonitorSlots refreshes the bookmark state of every populated monitor
// slot, mirroring updateBookmarkItem for the per-monitor menus.
func (tm *trayMenu) updateMonitorSlots() {
	for _, slot := range tm.monitorSlots {
		if slot.screenID == "" {
			continue
		}
		tm.updateSlotBookmark(slot)
	}
}

func (tm *trayMenu) updateSlotBookmark(slot *monitorSlot) {
	if slot.artworkID == "" {
		slot.bookmark.Disable()
		slot.bookmark.SetTitle("Bookmark Artwork")
		return
	}

	if !tm.ctrl.PixivLoggedIn() {
		slot.bookmark.Disable()
		slot.bookmark.SetTitle("Bookmark Artwork")
		return
	}

	if tm.ctrl.IsWallpaperBookmarked(slot.artworkID) {
		slot.bookmark.Disable()
		slot.bookmark.SetTitle("Bookmarked")
		return
	}

	slot.bookmark.Enable()
	slot.bookmark.SetTitle("Bookmark Artwork")
}
