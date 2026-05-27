package wallpaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateKDEScreenLockerConfigCreatesSectionOnEmptyInput(t *testing.T) {
	got := updateKDEScreenLockerConfig("", "file:///tmp/wall.jpg")

	if !strings.Contains(got, kdeGreeterSection) {
		t.Fatalf("missing greeter section in output: %q", got)
	}
	if !strings.Contains(got, "WallpaperPlugin=org.kde.image") {
		t.Fatalf("missing wallpaper plugin key in output: %q", got)
	}
	if !strings.Contains(got, kdeLockScreenSection) {
		t.Fatalf("missing lock screen section in output: %q", got)
	}
	if !strings.Contains(got, "Image=file:///tmp/wall.jpg") {
		t.Fatalf("missing image key in output: %q", got)
	}
}

func TestUpdateKDEScreenLockerConfigUpdatesGreeterPlugin(t *testing.T) {
	input := "[Greeter]\nWallpaperPlugin=org.kde.color\n[Greeter][Wallpaper][org.kde.color][General]\nColor=0,0,0\n"
	got := updateKDEScreenLockerConfig(input, "file:///new.jpg")

	if !strings.Contains(got, "WallpaperPlugin=org.kde.image") {
		t.Fatalf("greeter plugin was not updated: %q", got)
	}
	if !strings.Contains(got, "[Greeter][Wallpaper][org.kde.color][General]") {
		t.Fatalf("other wallpaper section should be preserved: %q", got)
	}
	if !strings.Contains(got, "Color=0,0,0") {
		t.Fatalf("other wallpaper settings should be preserved: %q", got)
	}
}

func TestUpdateKDEScreenLockerConfigInsertsGreeterPluginWhenMissing(t *testing.T) {
	input := "[Greeter]\nSomeOtherKey=true\n"
	got := updateKDEScreenLockerConfig(input, "file:///new.jpg")

	if !strings.Contains(got, "SomeOtherKey=true") {
		t.Fatalf("existing greeter keys should be preserved: %q", got)
	}
	if !strings.Contains(got, "WallpaperPlugin=org.kde.image") {
		t.Fatalf("greeter plugin key should be inserted: %q", got)
	}
}

func TestKDELockScreenUpdaterCreatesMissingConfigFile(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "kscreenlockerrc")

	updater := &KDELockScreenUpdater{configPath: configPath}
	if err := updater.UpdateImage("file:///tmp/new.jpg"); err != nil {
		t.Fatalf("UpdateImage() returned error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected config file to be created, read error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "WallpaperPlugin=org.kde.image") {
		t.Fatalf("missing greeter plugin setting in created config: %q", content)
	}
	if !strings.Contains(content, "Image=file:///tmp/new.jpg") {
		t.Fatalf("missing image setting in created config: %q", content)
	}
}

func TestUpdateKDEScreenLockerConfigUpdatesExistingImage(t *testing.T) {
	input := "[Greeter][Wallpaper][org.kde.image][General]\nImage=file:///old.jpg\n[Other]\nKey=Value\n"
	got := updateKDEScreenLockerConfig(input, "file:///new.jpg")

	if !strings.Contains(got, "Image=file:///new.jpg") {
		t.Fatalf("image key was not updated: %q", got)
	}
	if strings.Contains(got, "Image=file:///old.jpg") {
		t.Fatalf("old image key still present: %q", got)
	}
}

func TestUpdateKDEScreenLockerConfigAddsImageKeyToSection(t *testing.T) {
	input := "[Greeter][Wallpaper][org.kde.image][General]\nColor=255,255,255\n[Other]\nKey=Value\n"
	got := updateKDEScreenLockerConfig(input, "file:///new.jpg")

	if !strings.Contains(got, "Color=255,255,255") {
		t.Fatalf("existing section values should be preserved: %q", got)
	}
	if !strings.Contains(got, "Image=file:///new.jpg") {
		t.Fatalf("image key was not added: %q", got)
	}
}
