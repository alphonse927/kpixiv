package app

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func promptForInput(title, prompt string) (string, error) {
	if _, err := exec.LookPath("kdialog"); err == nil {
		cmd := exec.Command("kdialog", "--title", title, "--inputbox", prompt)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr != nil {
			return "", fmt.Errorf("pixiv login input was cancelled or failed: %w", runErr)
		}
		return strings.TrimSpace(stdout.String()), nil
	}

	if _, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.Command("zenity", "--entry", "--title", title, "--text", prompt)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr != nil {
			return "", fmt.Errorf("pixiv login input was cancelled or failed: %w", runErr)
		}
		return strings.TrimSpace(stdout.String()), nil
	}

	return "", fmt.Errorf("no desktop dialog tool found; install kdialog or zenity")
}

func showInfoDialog(title, message string) error {
	if _, err := exec.LookPath("kdialog"); err == nil {
		return exec.Command("kdialog", "--title", title, "--msgbox", message).Run()
	}

	if _, err := exec.LookPath("zenity"); err == nil {
		return exec.Command("zenity", "--info", "--title", title, "--text", message).Run()
	}

	return fmt.Errorf("no desktop dialog tool found; install kdialog or zenity")
}
