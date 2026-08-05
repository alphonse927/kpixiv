package wallpaper

import "testing"

func TestScreenLabel(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
		want   string
	}{
		{
			name:   "name and model",
			screen: Screen{ID: "DP-2", Name: "DP-2", Model: "DELL U2723QE"},
			want:   "DP-2 (DELL U2723QE)",
		},
		{
			name:   "name only",
			screen: Screen{ID: "DP-2", Name: "DP-2"},
			want:   "DP-2",
		},
		{
			name:   "model only",
			screen: Screen{ID: "eDP-1", Model: "LG UltraFine"},
			want:   "Screen eDP-1 (LG UltraFine)",
		},
		{
			name:   "fallback to id",
			screen: Screen{ID: "HDMI-A-1"},
			want:   "Screen HDMI-A-1",
		},
		{
			name:   "empty screen",
			screen: Screen{},
			want:   "Screen ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.screen.Label(); got != tt.want {
				t.Fatalf("Screen.Label() = %q, want %q", got, tt.want)
			}
		})
	}
}
