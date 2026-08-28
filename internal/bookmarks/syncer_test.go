package bookmarks

import (
	"testing"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/pixiv"
)

func TestFilterByOrientation(t *testing.T) {
	images := []pixiv.Image{
		{ID: "landscape", Width: 1920, Height: 1080},
		{ID: "portrait", Width: 1080, Height: 1920},
		{ID: "square", Width: 1000, Height: 1000},
	}

	tests := []struct {
		name        string
		orientation config.WallpaperOrientation
		wantIDs     []string
	}{
		{"any keeps everything", config.WallpaperAnyOrientation, []string{"landscape", "portrait", "square"}},
		{"landscape keeps landscape and square", config.WallpaperLandscapeOrientation, []string{"landscape", "square"}},
		{"portrait keeps only portrait", config.WallpaperPortraitOrientation, []string{"portrait"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByOrientation(images, tt.orientation)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d images, want %d (%v)", len(got), len(tt.wantIDs), got)
			}
			for i, img := range got {
				if img.ID != tt.wantIDs[i] {
					t.Errorf("image %d = %q, want %q", i, img.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestSyncerSyncOrientation(t *testing.T) {
	t.Run("single monitor uses global orientation", func(t *testing.T) {
		s := &Syncer{cfg: &config.Config{Wallpaper: config.WallpaperConfig{
			Orientation: config.WallpaperPortraitOrientation,
		}}}

		if got := s.syncOrientation(); got != config.WallpaperPortraitOrientation {
			t.Errorf("syncOrientation() = %q, want %q", got, config.WallpaperPortraitOrientation)
		}
	})

	t.Run("multi monitor all landscape restricts to landscape", func(t *testing.T) {
		s := &Syncer{cfg: &config.Config{Wallpaper: config.WallpaperConfig{
			MultiMonitorEnabled: true,
			Monitors: map[string]config.MonitorConfig{
				"a": {Orientation: config.WallpaperLandscapeOrientation},
				"b": {Orientation: config.WallpaperLandscapeOrientation},
			},
		}}}

		if got := s.syncOrientation(); got != config.WallpaperLandscapeOrientation {
			t.Errorf("syncOrientation() = %q, want %q", got, config.WallpaperLandscapeOrientation)
		}
	})

	t.Run("multi monitor mixed orientations allows any", func(t *testing.T) {
		s := &Syncer{cfg: &config.Config{Wallpaper: config.WallpaperConfig{
			MultiMonitorEnabled: true,
			Monitors: map[string]config.MonitorConfig{
				"a": {Orientation: config.WallpaperLandscapeOrientation},
				"b": {Orientation: config.WallpaperPortraitOrientation},
			},
		}}}

		if got := s.syncOrientation(); got != config.WallpaperAnyOrientation {
			t.Errorf("syncOrientation() = %q, want %q", got, config.WallpaperAnyOrientation)
		}
	})
}
