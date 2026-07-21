package wallpaper

import "testing"

func TestParseScreenIDs(t *testing.T) {
	screens := parseScreenIDs("KPIXIV_SCREEN:0;KPIXIV_SCREEN:1;KPIXIV_SCREEN:1;\n")
	if len(screens) != 2 {
		t.Fatalf("parseScreenIDs() returned %d screens, want 2", len(screens))
	}
	if screens[0].ID != "0" || screens[1].ID != "1" {
		t.Fatalf("unexpected screens: %#v", screens)
	}
}

func TestParseKScreenNames(t *testing.T) {
	output := "Output: 1 DP-3 uuid\n\tenabled\nOutput: 2 DP-2 uuid\n\tenabled\nOutput: 3 eDP-1 uuid\n\tdisabled\n"
	names := parseKScreenNames(output)
	if len(names) != 2 || names[0] != "DP-3" || names[1] != "DP-2" {
		t.Fatalf("unexpected display names: %#v", names)
	}
}
