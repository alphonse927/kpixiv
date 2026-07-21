package wallpaper

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type KDESetter struct {
	qdbus             string
	setLockScreen     bool
	lockScreenUpdater *KDELockScreenUpdater
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
func (k *KDESetter) Screens() ([]Screen, error) {
	if k.qdbus == "" {
		return nil, fmt.Errorf("qdbus binary not found (tried: qdbus, qdbus6, qdbus-qt5)")
	}

	// qdbus does not preserve newlines emitted by Plasma's print(), so use a
	// delimiter and screenCount instead of parsing one print call per line.
	script := `for (var i = 0; i < screenCount; i++) print("KPIXIV_SCREEN:" + i + ";");`
	output, err := k.evaluate(script)
	if err != nil {
		return nil, err
	}

	screens := parseScreenIDs(output)
	for i, name := range k.displayNames() {
		if i >= len(screens) {
			break
		}
		screens[i].Name = name
		screens[i].Model = k.monitorModel(name)
	}
	for i := range screens {
		if screens[i].Name == "" {
			screens[i].Name = "Screen " + screens[i].ID
		}
	}
	return screens, nil
}

func (k *KDESetter) displayNames() []string {
	binary, err := exec.LookPath("kscreen-doctor")
	if err != nil {
		return nil
	}
	output, err := exec.Command(binary, "-o").CombinedOutput() // #nosec G204 -- fixed desktop utility
	if err != nil {
		return nil
	}
	return parseKScreenNames(string(output))
}

func parseKScreenNames(output string) []string {
	output = regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(output, "")
	outputLines := strings.Split(output, "\n")
	var names []string
	for i, line := range outputLines {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "Output:" {
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
			names = append(names, fields[2])
		}
	}
	return names
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
	if _, err := k.evaluate(script); err != nil {
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
func (k *KDESetter) SetForScreen(screenID, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	if k.qdbus == "" {
		return fmt.Errorf("qdbus binary not found (tried: qdbus, qdbus6, qdbus-qt5)")
	}

	imageURI := strconv.Quote("file://" + absPath)
	screen := strconv.Quote(screenID)
	script := `var allDesktops = desktops();
for (var i = 0; i < allDesktops.length; i++) {
 var d = allDesktops[i];
 if (String(d.screen) !== ` + screen + `) continue;
 d.wallpaperPlugin = "org.kde.image";
 d.currentConfigGroup = ["Wallpaper", "org.kde.image", "General"];
 d.writeConfig("Image", ` + imageURI + `);
}`
	if _, err := k.evaluate(script); err != nil {
		return fmt.Errorf("failed to set wallpaper on screen %q via qdbus: %w", screenID, err)
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
