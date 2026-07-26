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

func TestParseKScreenOutputs(t *testing.T) {
	output := "Output: 1 DP-3 uuid\n\tenabled\nOutput: 2 DP-2 uuid\n\tenabled\nOutput: 3 eDP-1 uuid\n\tdisabled\n"
	names := parseKScreenOutputs(output)
	if len(names) != 2 || names[1] != "DP-3" || names[2] != "DP-2" {
		t.Fatalf("unexpected display names: %#v", names)
	}
}

func TestParseKScreenOutputsUnordered(t *testing.T) {
	output := "Output: 2 DP-2 uuid\n\tenabled\nOutput: 1 DP-1 uuid\n\tenabled\n"
	names := parseKScreenOutputs(output)
	if len(names) != 2 || names[1] != "DP-1" || names[2] != "DP-2" {
		t.Fatalf("unexpected display names: %#v", names)
	}
}

func TestParsePlasmaScreenInfo(t *testing.T) {
	output := "KPIXIV_SCR:0:0,0,1920,1080;KPIXIV_SCR:1:1920,0,2560,1440;"
	screens := parsePlasmaScreenInfo(output)
	if len(screens) != 2 {
		t.Fatalf("parsePlasmaScreenInfo() returned %d screens, want 2", len(screens))
	}
	if screens[0].Index != 0 || screens[0].X != 0 || screens[0].Y != 0 || screens[0].Width != 1920 || screens[0].Height != 1080 {
		t.Fatalf("unexpected screen 0: %#v", screens[0])
	}
	if screens[1].Index != 1 || screens[1].X != 1920 || screens[1].Y != 0 || screens[1].Width != 2560 || screens[1].Height != 1440 {
		t.Fatalf("unexpected screen 1: %#v", screens[1])
	}
}

func TestParsePlasmaScreenInfoEmpty(t *testing.T) {
	screens := parsePlasmaScreenInfo("some unrelated text")
	if len(screens) != 0 {
		t.Fatalf("expected 0 screens, got %d", len(screens))
	}
}

func TestParsePlasmaScreenInfoDeduplicates(t *testing.T) {
	output := "KPIXIV_SCR:0:0,0,1920,1080;KPIXIV_SCR:0:0,0,1920,1080;"
	screens := parsePlasmaScreenInfo(output)
	if len(screens) != 1 {
		t.Fatalf("expected 1 screen (deduplicated), got %d", len(screens))
	}
}

func TestParsePlasmaScreenInfoTrailingNewline(t *testing.T) {
	// qdbus appends a trailing newline to the output, which previously caused
	// the height of the last screen to be parsed as "2560;" (with trailing
	// semicolon), resulting in atoi("2560;") -> 0.
	output := "KPIXIV_SCR:0:0,417,2560,1440;KPIXIV_SCR:1:2560,0,1440,2560;\n"
	screens := parsePlasmaScreenInfo(output)
	if len(screens) != 2 {
		t.Fatalf("expected 2 screens, got %d", len(screens))
	}
	if screens[1].Height != 2560 {
		t.Fatalf("screen 1 height = %d, want 2560", screens[1].Height)
	}
	if screens[0].Height != 1440 {
		t.Fatalf("screen 0 height = %d, want 1440", screens[0].Height)
	}
}

func TestParseKScreenOutputsFull(t *testing.T) {
	output := "Output: 1 DP-3 uuid\n\tenabled\n\tgeometry: 0,0 2560x1440\nOutput: 2 DP-2 uuid\n\tenabled\n\tgeometry: 2560,0 1920x1080\nOutput: 3 eDP-1 uuid\n\tdisabled\n"
	outputs := parseKScreenOutputsFull(output)
	if len(outputs) != 2 {
		t.Fatalf("parseKScreenOutputsFull() returned %d outputs, want 2", len(outputs))
	}
	if outputs[0].Connector != "DP-3" || outputs[0].X != 0 || outputs[0].Y != 0 || outputs[0].Width != 2560 || outputs[0].Height != 1440 || !outputs[0].Enabled {
		t.Fatalf("unexpected output 0: %#v", outputs[0])
	}
	if outputs[1].Connector != "DP-2" || outputs[1].X != 2560 || outputs[1].Y != 0 || outputs[1].Width != 1920 || outputs[1].Height != 1080 || !outputs[1].Enabled {
		t.Fatalf("unexpected output 1: %#v", outputs[1])
	}
}

func TestParseKScreenOutputsFullUnorderedIDs(t *testing.T) {
	// Output IDs 10 and 5 — deliberately non-sequential to validate that
	// the parser does not assume any relationship between IDs and screen indices.
	output := "Output: 10 HDMI-A-1 uuid\n\tenabled\n\tgeometry: 0,0 1920x1080\nOutput: 5 DP-1 uuid\n\tenabled\n\tgeometry: 1920,0 2560x1440\n"
	outputs := parseKScreenOutputsFull(output)
	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}
	// Results should preserve parse order.
	if outputs[0].Connector != "HDMI-A-1" || outputs[1].Connector != "DP-1" {
		t.Fatalf("unexpected connector order: %s, %s", outputs[0].Connector, outputs[1].Connector)
	}
	if outputs[0].ID != 10 || outputs[1].ID != 5 {
		t.Fatalf("unexpected output IDs: %d, %d", outputs[0].ID, outputs[1].ID)
	}
}

func TestParseKScreenOutputsFullDisabled(t *testing.T) {
	output := "Output: 1 DP-1 uuid\n\tenabled\n\tgeometry: 0,0 1920x1080\nOutput: 2 eDP-1 uuid\n\tdisabled\n\tgeometry: 1920,0 2560x1440\nOutput: 3 DP-2 uuid\n\tenabled\n\tgeometry: 1920,0 2560x1440\n"
	outputs := parseKScreenOutputsFull(output)
	if len(outputs) != 2 {
		t.Fatalf("expected 2 enabled outputs, got %d", len(outputs))
	}
	if outputs[0].Connector != "DP-1" || outputs[1].Connector != "DP-2" {
		t.Fatalf("unexpected connectors: %s, %s", outputs[0].Connector, outputs[1].Connector)
	}
}

func TestParseKScreenOutputsFullCapitalGeometry(t *testing.T) {
	// kscreen-doctor -o outputs "Geometry:" with a capital G.
	output := "Output: 1 DP-2 uuid\n\tenabled\n\tGeometry: 2560,0 1440x2560\nOutput: 2 DP-1 uuid\n\tenabled\n\tGeometry: 0,417 2560x1440\n"
	outputs := parseKScreenOutputsFull(output)
	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}
	if outputs[0].Connector != "DP-2" || outputs[0].X != 2560 || outputs[0].Y != 0 || outputs[0].Width != 1440 || outputs[0].Height != 2560 {
		t.Fatalf("unexpected output 0: %#v", outputs[0])
	}
	if outputs[1].Connector != "DP-1" || outputs[1].X != 0 || outputs[1].Y != 417 || outputs[1].Width != 2560 || outputs[1].Height != 1440 {
		t.Fatalf("unexpected output 1: %#v", outputs[1])
	}
}
