package protocol

import (
	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

type Controller interface {
	Start() error
	Monitors() ([]wallpaper.Screen, error)
	MonitorWallpapers() (map[string]*storage.ImageMeta, error)
	NextWallpaper() error
	NextWallpaperForMonitor(monitorID string) error
	NextWallpaperForAllMonitors() error
	BookmarkCurrentArtwork() error
	ExcludeCurrentWallpaper() error
	OpenCurrentArtwork() error
	OpenCurrentArtworkInPixiv() error
	CopyCurrentArtwork() error
	BookmarkWallpaper(artworkID string) error
	ExcludeWallpaper(artworkID string) error
	OpenWallpaperFile(artworkID string) error
	OpenWallpaperInPixiv(artworkID string) error
	CopyWallpaperToFavorites(artworkID string) error
	PauseRotation()
	ResumeRotation()
	PixivLoggedIn() bool
	PixivUserName() string
	IsArtworkBookmarked() bool
	MultiMonitorEnabled() bool
	LoginToPixiv() error
	LogoutFromPixiv() error
	ShowSettingsWindow() error
	ShowAccountSettings() error
	Shutdown()
	Config() *config.Config
}
