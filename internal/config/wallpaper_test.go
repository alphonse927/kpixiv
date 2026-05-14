package config

import "testing"

func TestWallpaperModeString(t *testing.T) {
	tests := []struct {
		mode     WallpaperMode
		expected string
	}{
		{WallpaperFillMode, "fill"},
		{WallpaperCoverMode, "cover"},
		{WallpaperFitMode, "fit"},
		{WallpaperMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("WallpaperMode.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}
