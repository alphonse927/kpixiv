package gui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"github.com/alphonse927/kpixiv/internal/auth"
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

const (
	HomePage = iota
	SettingsPage
	MonitorPage
	AccountPage
	AboutPage
)

type AppController interface {
	Config() *config.Config
	ApplyConfig(cfg *config.Config)
	NextWallpaper() error
	FetchNow() error
	PixivLoggedIn() bool
	PixivUserName() string
	AutoLogin(ctx context.Context, cfg auth.LoginConfig) error
	LogoutFromPixiv() error
	SchedulerRunning() bool
	CurrentWallpaper() (*storage.ImageMeta, error)
	CachedCount() int
	CacheStats() (storage.CacheStats, error)
	HistoryCount() int
	NextWallpaperID() string
	WallpaperMeta(id string) (*storage.ImageMeta, error)
	LastRotation() time.Time
	LastFetch() time.Time
	LastBookmarkSync() time.Time
	FetchInProgress() bool
	LastFetchAttempt() time.Time
	LastFetchError() error
	BookmarkSyncInProgress() bool
	LastBookmarkSyncAttempt() time.Time
	LastBookmarkSyncError() error
	ServiceEnabled() (bool, error)
	EnableService() error
	DisableService() error
	ThumbnailPath(id string) string
	EnsureThumbnail(id string) string
	Monitors() ([]wallpaper.Screen, error)
	MonitorWallpapers() (map[string]*storage.ImageMeta, error)
	MonitorNextWallpapers() map[string]string
	ConfigPath() string
	DataDir() string
	StateDir() string
	DownloadDir() string
}

var (
	guiApp    fyne.App
	settingsW *settingsUI
)

func Run(ctrl AppController, ctx context.Context, quitCh <-chan struct{}) {
	a := app.NewWithID("kpixiv")
	a.Settings().SetTheme(&tintedBG{theme.DefaultTheme()})

	guiApp = a
	settingsW = newSettingsUI(a, ctrl, logger.WithComponent("settings")) //nolint:contextcheck // settingsUI does not require a context

	if r, ok := ctrl.(interface{ SetGUIRefresher(func()) }); ok {
		r.SetGUIRefresher(func() {
			fyne.Do(func() {
				if settingsW != nil {
					settingsW.rebuildAccountPage()
				}
			})
		})
	}

	go func() {
		select {
		case <-quitCh:
		case <-ctx.Done():
		}
		fyne.Do(a.Quit)
	}()

	a.Run()
}

func ShowSettings(ctrl AppController, page int) {
	if guiApp == nil {
		return
	}
	fyne.Do(func() {
		settingsW.ctrl = ctrl
		settingsW.update()
		settingsW.show()
		settingsW.selectPage(page)
	})
}
