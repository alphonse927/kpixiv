package wallpaper

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type KDESetter struct {
	qdbus             string
	setLockScreen     bool
	lockScreenUpdater *KDELockScreenUpdater
	primaryScreenIdx  string // Plasma screen index of the primary display, cached from Screens()
}

// plasmaScreenInfo holds the index and geometry of one Plasma screen.
type plasmaScreenInfo struct {
	Index               int
	X, Y, Width, Height int
}

// KScreenOutputInfo holds parsed data for one KScreen output.
type KScreenOutputInfo struct {
	ID            int
	Connector     string
	Model         string
	Enabled       bool
	Primary       bool
	X, Y          int
	Width, Height int
}

// NewKDESetter creates a KDE wallpaper setter using qdbus.
func NewKDESetter(setLockScreen bool) *KDESetter {
	qdbus := detectQDBusBinary()
	return &KDESetter{
		qdbus:             qdbus,
		setLockScreen:     setLockScreen,
		lockScreenUpdater: NewKDELockScreenUpdater(),
	}
}

// Screens returns the physical screens currently known to Plasma.
// It maps Plasma screen indices to KScreen connector names by sorting
// KScreen outputs by their physical position (left-to-right, top-to-bottom)
// and mapping them by index.  This avoids fragile assumptions about output
// IDs or geometry equality, and works correctly even with identical monitors.
func (k *KDESetter) Screens() ([]Screen, error) {
	if k.qdbus == "" {
		return nil, fmt.Errorf("qdbus binary not found (tried: qdbus, qdbus6, qdbus-qt5)")
	}

	script := `for (var i = 0; i < screenCount; i++) print("KPIXIV_SCREEN:" + i + ";");`
	output, err := k.evaluate(script)
	if err != nil {
		return nil, err
	}

	plasmaIndices := parseScreenIDs(output)

	kscreenOutputs := k.getKScreenOutputsFull()

	// Sort KScreen outputs by position: left-to-right, then top-to-bottom.
	// This matches how KWin assigns Plasma screen indices.
	sort.Slice(kscreenOutputs, func(i, j int) bool {
		if kscreenOutputs[i].X != kscreenOutputs[j].X {
			return kscreenOutputs[i].X < kscreenOutputs[j].X
		}
		return kscreenOutputs[i].Y < kscreenOutputs[j].Y
	})

	screens := make([]Screen, 0, len(plasmaIndices))
	k.primaryScreenIdx = ""
	for _, ps := range plasmaIndices {
		idx := atoiDefault(ps.ID, -1)
		s := Screen{
			Index: ps.ID,
			Name:  "Screen " + ps.ID,
		}
		if idx >= 0 && idx < len(kscreenOutputs) {
			ko := kscreenOutputs[idx]
			s.ID = ko.Connector
			s.Name = ko.Connector
			s.Model = ko.Model
			s.Primary = ko.Primary
		} else {
			s.ID = ps.ID
		}

		if s.Primary {
			k.primaryScreenIdx = s.Index
		}

		screens = append(screens, s)
	}

	return screens, nil
}

// getKScreenOutputsFull runs kscreen-doctor -o and parses each output's
// connector, geometry, and enabled status.
func (k *KDESetter) getKScreenOutputsFull() []KScreenOutputInfo {
	binary, err := exec.LookPath("kscreen-doctor")
	if err != nil {
		return nil
	}

	output, err := exec.Command(binary, "-o").CombinedOutput() // #nosec G204
	if err != nil {
		return nil
	}

	outputs := parseKScreenOutputsFull(string(output))

	// Enrich with model names from EDID.
	for i := range outputs {
		outputs[i].Model = k.monitorModel(outputs[i].Connector)
	}

	return outputs
}

// parseKScreenOutputsFull parses kscreen-doctor -o output, extracting the
// connector name, geometry (position + size), and enabled status for each
// output. Unlike parseKScreenOutputs, this does NOT assume any particular
// relationship between output IDs and screen indices.
func parseKScreenOutputsFull(output string) []KScreenOutputInfo {
	output = regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(output, "")
	lines := strings.Split(output, "\n")

	var results []KScreenOutputInfo
	var cur *KScreenOutputInfo

	flushCur := func() {
		if cur != nil && cur.Enabled {
			results = append(results, *cur)
		}
		cur = nil
	}

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "Output:" {
			flushCur()
			num := atoiDefault(fields[1], 0)
			cur = &KScreenOutputInfo{
				ID:        num,
				Connector: fields[2],
				Enabled:   false,
			}
			continue
		}

		if cur == nil {
			continue
		}

		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "enabled":
			cur.Enabled = true
		case trimmed == "disabled":
			cur.Enabled = false
			flushCur()
		case strings.HasPrefix(strings.ToLower(trimmed), "priority:"):
			prioStr := strings.TrimSpace(trimmed[9:])
			prio := atoiDefault(prioStr, 0)
			cur.Primary = prio == 1
		case strings.HasPrefix(strings.ToLower(trimmed), "geometry:"):
			geo := strings.TrimSpace(trimmed[9:])
			// Format: "x,y widthxheight" or "x,y,widthxheight"
			parts := strings.Split(geo, " ")
			if len(parts) == 2 {
				xy := strings.Split(parts[0], ",")
				wh := strings.Split(parts[1], "x")
				if len(xy) == 2 {
					cur.X = atoiDefault(strings.TrimSpace(xy[0]), 0)
					cur.Y = atoiDefault(strings.TrimSpace(xy[1]), 0)
				}
				if len(wh) == 2 {
					cur.Width = atoiDefault(strings.TrimSpace(wh[0]), 0)
					cur.Height = atoiDefault(strings.TrimSpace(wh[1]), 0)
				}
			}
		}
	}

	flushCur()
	return results
}

// parsePlasmaScreenInfo parses the output of the screenGeometries() query.
// Expected token format: KPIXIV_SCR:<index>:<x>,<y>,<width>,<height>;
func parsePlasmaScreenInfo(output string) []plasmaScreenInfo {
	var screens []plasmaScreenInfo
	seen := make(map[int]bool)

	for _, token := range strings.Split(output, "KPIXIV_SCR:") {
		token = strings.TrimRight(strings.TrimSpace(token), ";")
		if token == "" {
			continue
		}

		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 {
			continue
		}

		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if seen[idx] {
			continue
		}
		seen[idx] = true

		coords := strings.Split(parts[1], ",")
		if len(coords) != 4 {
			continue
		}

		screens = append(screens, plasmaScreenInfo{
			Index:  idx,
			X:      atoiDefault(coords[0], 0),
			Y:      atoiDefault(coords[1], 0),
			Width:  atoiDefault(coords[2], 0),
			Height: atoiDefault(coords[3], 0),
		})
	}

	return screens
}

// parseKScreenOutputs parses kscreen-doctor -o output and returns a map of
// output ID -> connector name for enabled outputs. Deprecated: use
// parseKScreenOutputsFull instead (which includes geometry).
func parseKScreenOutputs(output string) map[int]string {
	output = regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(output, "")
	outputLines := strings.Split(output, "\n")
	outputs := make(map[int]string)

	for i, line := range outputLines {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "Output:" {
			continue
		}

		outputNum, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		enabled := false
		for _, stateLine := range outputLines[i+1:] {
			stateFields := strings.Fields(stateLine)
			if len(stateFields) > 0 && stateFields[0] == "Output:" {
				break
			}

			if len(stateFields) == 1 && stateFields[0] == "enabled" {
				enabled = true
			}

			if len(stateFields) == 1 && stateFields[0] == "disabled" {
				break
			}
		}

		if enabled {
			outputs[outputNum] = fields[2]
		}
	}

	return outputs
}

func parseScreenIDs(output string) []Screen {
	var screens []Screen
	seen := make(map[string]struct{})
	for _, token := range strings.Split(output, "KPIXIV_SCREEN:") {
		id := strings.TrimSuffix(strings.TrimSpace(token), ";")
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		screens = append(screens, Screen{ID: id})
	}
	return screens
}

// Set applies the wallpaper on KDE Plasma and optionally lock screen.
func (k *KDESetter) Set(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	if k.qdbus == "" {
		return fmt.Errorf("qdbus binary not found (tried: qdbus, qdbus6, qdbus-qt5)")
	}

	// Escape for JavaScript string literal context.
	imageURI := strconv.Quote("file://" + absPath)

	script := `var allDesktops = desktops();

for (var i = 0; i < allDesktops.length; i++) {
	var d = allDesktops[i];

	d.wallpaperPlugin = "org.kde.image";
	d.currentConfigGroup = ["Wallpaper", "org.kde.image", "General"];
	d.writeConfig("Image", ` + imageURI + `);
}`

	// #nosec G204 -- script is constructed safely using strconv.Quote; qdbus is a trusted KDE Plasma interface
	if _, err = k.evaluate(script); err != nil {
		return fmt.Errorf("failed to set wallpaper via qdbus: %w", err)
	}

	if !k.setLockScreen {
		// No need to update the lock screen wallpaper.
		return nil
	}

	// Update the lock screen wallpaper.
	imageURI = (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String()
	if updateErr := k.lockScreenUpdater.UpdateImage(imageURI); updateErr != nil {
		return fmt.Errorf("failed to update KDE lock screen wallpaper: %w", updateErr)
	}

	return nil
}

// SetForScreen applies a wallpaper to every virtual desktop on one screen.
// screenIndex must be the Plasma screen index (e.g. "0"), NOT the connector
// name — it is used directly in the qdbus JavaScript d.screen comparison.
func (k *KDESetter) SetForScreen(screenIndex, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	if k.qdbus == "" {
		return fmt.Errorf("qdbus binary not found (tried: qdbus, qdbus6, qdbus-qt5)")
	}

	imageURI := strconv.Quote("file://" + absPath)
	screen := strconv.Quote(screenIndex)
	script := `var allDesktops = desktops();
for (var i = 0; i < allDesktops.length; i++) {
 var d = allDesktops[i];
 if (String(d.screen) !== ` + screen + `) continue;
 d.wallpaperPlugin = "org.kde.image";
 d.currentConfigGroup = ["Wallpaper", "org.kde.image", "General"];
 d.writeConfig("Image", ` + imageURI + `);
}`
	if _, err = k.evaluate(script); err != nil {
		return fmt.Errorf("failed to set wallpaper on screen %q via qdbus: %w", screenIndex, err)
	}

	if !k.setLockScreen || screenIndex != k.primaryScreenIdx {
		return nil
	}

	imageURI = (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String()
	if updateErr := k.lockScreenUpdater.UpdateImage(imageURI); updateErr != nil {
		return fmt.Errorf("failed to update KDE lock screen wallpaper: %w", updateErr)
	}

	return nil
}

func (k *KDESetter) evaluate(script string) (string, error) {
	cmd := exec.Command(k.qdbus, "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", script) // #nosec G204
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("qdbus evaluateScript: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}

func atoiDefault(s string, defaultVal int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return defaultVal
	}
	return v
}

func detectQDBusBinary() string {
	candidates := []string{"qdbus", "qdbus6", "qdbus-qt5"}
	for _, name := range candidates {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

// monitorModel reads the EDID for the given DRM connector and returns the
// monitor model (e.g. "DELL S2721DS"). Returns "" on failure.
func (k *KDESetter) monitorModel(connector string) string {
	matches, err := filepath.Glob("/sys/class/drm/card*-" + connector + "/edid")
	if err != nil || len(matches) == 0 {
		return ""
	}
	if m := readModelEDIDDecode(matches[0]); m != "" {
		return m
	}
	return readModelRaw(matches[0])
}

func readModelEDIDDecode(path string) string {
	if _, err := exec.LookPath("edid-decode"); err != nil {
		return ""
	}
	out, err := exec.Command("edid-decode", path).CombinedOutput() // #nosec G204
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`Display Product Name: '([^']+)'`)
	m := re.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func readModelRaw(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	// Pick the longest plausible ASCII string from the raw EDID blob.
	var best, cur string
	for _, b := range data {
		if b >= 0x20 && b < 0x7f {
			cur += string(b)
		} else {
			if len(cur) > len(best) {
				best = cur
			}
			cur = ""
		}
	}

	if len(cur) > len(best) {
		best = cur
	}

	if len(best) > 4 {
		return strings.TrimSpace(best)
	}

	return ""
}
