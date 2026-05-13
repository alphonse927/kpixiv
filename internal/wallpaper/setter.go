package wallpaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kpixiv/kpixiv/internal/logger"
)

type Setter interface {
	Set(path string) error
}

type KDESetter struct {
	screen string
}

func NewKDESetter() *KDESetter {
	return &KDESetter{
		screen: "0",
	}
}

func (k *KDESetter) Set(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	cmd := exec.Command("qdbus", "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", `
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

type KDEBackgroundSetter struct{}

func NewKDEBackgroundSetter() *KDEBackgroundSetter {
	return &KDEBackgroundSetter{}
}

func (k *KDEBackgroundSetter) Set(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	script := fmt.Sprintf(`
		var wallpaper = "%s";
		var Desktops = desktops();
		for (var i = 0; i < Desktops.length; i++) {
			var d = Desktops[i];
			d.wallpaperPlugin = "org.kde.image";
			d.writeConfig("Image", "file://" + wallpaper);
		}
	`, strings.ReplaceAll(absPath, "\\", "\\\\"))

	cmd := exec.Command("qdbus", "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set wallpaper: %w (output: %s)", err, string(output))
	}

	return nil
}

type GNOMESetter struct{}

func NewGNOMESetter() *GNOMESetter {
	return &GNOMESetter{}
}

func (g *GNOMESetter) Set(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	cmd := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", fmt.Sprintf("file://%s", absPath))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	return nil
}

type BGSetter struct{}

type DryRunSetter struct{}

func NewDryRunSetter() *DryRunSetter {
	return &DryRunSetter{}
}

func (d *DryRunSetter) Set(path string) error {
	log := logger.WithComponent("wallpaper")
	log.Info("Dry-run: skipping wallpaper apply", "path", path)
	return nil
}

func (b *BGSetter) Set(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("wallpaper file does not exist: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	detectDesktop := func() Setter {
		if _, err := exec.LookPath("qdbus"); err == nil {
			if out, _ := exec.Command("qdbus", "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", "desktops().length").Output(); strings.Contains(string(out), "KDE") || string(out) != "" {
				return NewKDEBackgroundSetter()
			}
		}
		if _, err := exec.LookPath("gsettings"); err == nil {
			return NewGNOMESetter()
		}
		return nil
	}

	setter := detectDesktop()
	if setter == nil {
		return fmt.Errorf("no supported desktop environment detected")
	}

	return setter.Set(absPath)
}
