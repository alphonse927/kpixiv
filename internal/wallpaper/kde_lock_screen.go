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
	kdeGreeterSection    = "[Greeter]"
	kdePluginKey         = "WallpaperPlugin="
	kdeImagePlugin       = "org.kde.image"
)

type KDELockScreenUpdater struct {
	configPath string
}

func NewKDELockScreenUpdater() *KDELockScreenUpdater {
	return &KDELockScreenUpdater{configPath: kdeScreenLockerConfigPath()}
}

func kdeScreenLockerConfigPath() string {
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, "kscreenlockerrc")
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".config", "kscreenlockerrc")
	}

	// Last-resort fallback for unusual environments.
	return filepath.Join(".config", "kscreenlockerrc")
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

	// #nosec G304,G703 -- path is resolved from the current user's config directory.
	if writeErr := os.WriteFile(u.configPath, []byte(updated), 0600); writeErr != nil {
		return fmt.Errorf("failed to write %s: %w", u.configPath, writeErr)
	}

	return nil
}

func updateKDEScreenLockerConfig(content, imageURI string) string {
	if strings.TrimSpace(content) == "" {
		return kdeGreeterSection + "\n" +
			kdePluginKey + kdeImagePlugin + "\n\n" +
			kdeLockScreenSection + "\n" +
			kdeLockScreenKey + imageURI + "\n"
	}

	hadTrailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")

	lines = upsertSectionKey(lines, kdeGreeterSection, kdePluginKey, kdeImagePlugin)
	lines = upsertSectionKey(lines, kdeLockScreenSection, kdeLockScreenKey, imageURI)

	result := strings.Join(lines, "\n")
	if hadTrailingNewline && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return result
}

func upsertSectionKey(lines []string, sectionName, keyPrefix, value string) []string {
	sectionStart := -1
	sectionEnd := len(lines)

	for i, line := range lines {
		lineTrim := strings.TrimSpace(line)
		if lineTrim != sectionName {
			continue
		}

		sectionStart = i
		for j := i + 1; j < len(lines); j++ {
			nextTrim := strings.TrimSpace(lines[j])
			if strings.HasPrefix(nextTrim, "[") && strings.HasSuffix(nextTrim, "]") {
				sectionEnd = j
				break
			}
		}
		break
	}

	newEntry := keyPrefix + value

	if sectionStart == -1 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, sectionName, newEntry)
		return lines
	}

	for i := sectionStart + 1; i < sectionEnd; i++ {
		lineTrim := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(lineTrim, keyPrefix) {
			continue
		}

		idx := strings.Index(lines[i], keyPrefix)
		if idx >= 0 {
			lines[i] = lines[i][:idx] + newEntry
		} else {
			lines[i] = newEntry
		}
		return lines
	}

	before := append([]string{}, lines[:sectionEnd]...)
	after := append([]string{}, lines[sectionEnd:]...)
	before = append(before, newEntry)
	return append(before, after...)
}
