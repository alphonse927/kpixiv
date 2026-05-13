package wallpaper

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type KDESetter struct {
	screen string
	qdbus  string
}

func NewKDESetter() *KDESetter {
	qdbus := detectQDBusBinary()
	return &KDESetter{
		screen: "0",
		qdbus:  qdbus,
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

	cmd := exec.Command(k.qdbus, "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", `
		var allDesktops = desktops();
		for (i=0;i<allDesktops.length;i++) {
			d = allDesktops[i];
			d.wallpaperPlugin = "org.kde.image";
			d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
			d.writeConfig("Image", "file://`+absPath+`");
		}
	`)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set wallpaper via qdbus: %w (output: %s)", err, string(output))
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

func isKDEAvailable() bool {
	qdbus := detectQDBusBinary()
	if qdbus == "" {
		return false
	}
	out, err := exec.Command(qdbus, "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", "desktops().length").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
