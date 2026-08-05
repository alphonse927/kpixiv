package tray

import (
	"testing"

	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

func TestBuildSlotData(t *testing.T) {
	screens := []wallpaper.Screen{
		{ID: "DP-2", Name: "DP-2", Model: "DELL U2723QE", Primary: true},
		{ID: "eDP-1"},
	}

	wallpapers := map[string]*storage.ImageMeta{
		"DP-2":  {ID: "100", Title: "Morning", Artist: "ArtistA"},
		"eDP-1": {ID: "200"},
	}

	slots := buildSlotData(screens, wallpapers)
	if len(slots) != 2 {
		t.Fatalf("buildSlotData() returned %d slots, want 2", len(slots))
	}

	if slots[0].screen.ID != "DP-2" || slots[0].meta.ID != "100" {
		t.Fatalf("unexpected slot 0: %#v", slots[0])
	}
	if slots[1].screen.ID != "eDP-1" || slots[1].meta.ID != "200" {
		t.Fatalf("unexpected slot 1: %#v", slots[1])
	}
}

func TestBuildSlotDataMissingWallpaper(t *testing.T) {
	screens := []wallpaper.Screen{{ID: "DP-2"}}

	slots := buildSlotData(screens, nil)
	if len(slots) != 1 {
		t.Fatalf("buildSlotData() returned %d slots, want 1", len(slots))
	}
	if slots[0].meta != nil {
		t.Fatalf("screen without wallpaper should have nil meta, got %#v", slots[0].meta)
	}
}

func TestBuildSlotDataPreservesOrder(t *testing.T) {
	screens := []wallpaper.Screen{
		{ID: "DP-1"},
		{ID: "HDMI-A-1"},
		{ID: "eDP-1"},
	}
	wallpapers := map[string]*storage.ImageMeta{
		"eDP-1": {ID: "300"},
		"DP-1":  {ID: "100"},
	}

	slots := buildSlotData(screens, wallpapers)
	for i, want := range []string{"DP-1", "HDMI-A-1", "eDP-1"} {
		if slots[i].screen.ID != want {
			t.Fatalf("slot %d screen = %q, want %q", i, slots[i].screen.ID, want)
		}
	}
	if slots[1].meta != nil {
		t.Fatalf("HDMI-A-1 has no wallpaper, want nil meta, got %#v", slots[1].meta)
	}
}

func TestSlotHeaderTitle(t *testing.T) {
	tests := []struct {
		name string
		meta *storage.ImageMeta
		want string
	}{
		{name: "nil meta", meta: nil, want: "No wallpaper set"},
		{name: "title and artist", meta: &storage.ImageMeta{ID: "100", Title: "Morning", Artist: "ArtistA"}, want: "Morning by ArtistA"},
		{name: "title only", meta: &storage.ImageMeta{ID: "100", Title: "Morning"}, want: "Morning"},
		{name: "artist only", meta: &storage.ImageMeta{ID: "100", Artist: "ArtistA"}, want: "100 by ArtistA"},
		{name: "no title or artist", meta: &storage.ImageMeta{ID: "100"}, want: "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slotHeaderTitle(tt.meta); got != tt.want {
				t.Fatalf("slotHeaderTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScreenSubmenuTitle(t *testing.T) {
	tests := []struct {
		name   string
		screen wallpaper.Screen
		want   string
	}{
		{name: "primary with label", screen: wallpaper.Screen{ID: "DP-2", Name: "DP-2", Primary: true}, want: "DP-2 (Primary)"},
		{name: "non-primary", screen: wallpaper.Screen{ID: "eDP-1", Name: "eDP-1"}, want: "eDP-1"},
		{name: "primary fallback id", screen: wallpaper.Screen{ID: "HDMI-A-1", Primary: true}, want: "Screen HDMI-A-1 (Primary)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := screenSubmenuTitle(tt.screen); got != tt.want {
				t.Fatalf("screenSubmenuTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
