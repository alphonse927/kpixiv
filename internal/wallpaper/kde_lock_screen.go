package wallpaper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	kdeLockScreenSection = "[Greeter][Wallpaper][org.kde.image][General]"
	kdeLockScreenKey     = "Image="
)

type KDELockScreenUpdater struct {
	configPath string
}

func NewKDELockScreenUpdater() *KDELockScreenUpdater {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	return &KDELockScreenUpdater{
		configPath: filepath.Join(homeDir, ".config", "kscreenlockerrc"),
	}
}

func (u *KDELockScreenUpdater) UpdateImage(imageURI string) error {
	if !strings.HasPrefix(imageURI, "file://") {
		return fmt.Errorf("invalid image URI: %s", imageURI)
	}

	content, err := os.ReadFile(u.configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", u.configPath, err)
	}

	updated := updateKDEScreenLockerConfig(string(content), imageURI)

	dir := filepath.Dir(u.configPath)
	if mkErr := os.MkdirAll(dir, 0750); mkErr != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, mkErr)
	}

	if writeErr := os.WriteFile(u.configPath, []byte(updated), 0600); writeErr != nil {
		return fmt.Errorf("failed to write %s: %w", u.configPath, writeErr)
	}

	return nil
}

func updateKDEScreenLockerConfig(content, imageURI string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return kdeLockScreenSection + "\n" + kdeLockScreenKey + imageURI + "\n"
	}

	lines := strings.Split(content, "\n")
	sectionStart := -1
	sectionEnd := len(lines)

	for i, line := range lines {
		if strings.TrimSpace(line) == kdeLockScreenSection {
			sectionStart = i
			for j := i + 1; j < len(lines); j++ {
				lineTrim := strings.TrimSpace(lines[j])
				if strings.HasPrefix(lineTrim, "[") && strings.HasSuffix(lineTrim, "]") {
					sectionEnd = j
					break
				}
			}
			break
		}
	}

	if sectionStart == -1 {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += kdeLockScreenSection + "\n"
		content += kdeLockScreenKey + imageURI + "\n"
		return content
	}

	hasImageKey := false
	for i := sectionStart + 1; i < sectionEnd; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, kdeLockScreenKey) {
			lines[i] = kdeLockScreenKey + imageURI
			hasImageKey = true
			break
		}
	}

	if !hasImageKey {
		before := append([]string{}, lines[:sectionEnd]...)
		after := append([]string{}, lines[sectionEnd:]...)
		before = append(before, kdeLockScreenKey+imageURI)
		lines = append(before, after...)
	}

	return strings.Join(lines, "\n")
}
