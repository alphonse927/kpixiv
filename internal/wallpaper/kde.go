package wallpaper

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
)

type KDESetter struct {
	screen            string
	qdbus             string
	setLockScreen     bool
	lockScreenUpdater *KDELockScreenUpdater
}

func NewKDESetter(setLockScreen bool) *KDESetter {
	qdbus := detectQDBusBinary()
	return &KDESetter{
		screen:            "0",
		qdbus:             qdbus,
		setLockScreen:     setLockScreen,
		lockScreenUpdater: NewKDELockScreenUpdater(),
	}
}

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
	cmd := exec.Command(
		k.qdbus,
		"org.kde.plasmashell",
		"/PlasmaShell",
		"org.kde.PlasmaShell.evaluateScript",
		script,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"failed to set wallpaper via qdbus: %w (output: %s)",
			err,
			string(output),
		)
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

func detectQDBusBinary() string {
	candidates := []string{"qdbus", "qdbus6", "qdbus-qt5"}
	for _, name := range candidates {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}
