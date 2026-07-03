package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

func IsServiceEnabled(service string) (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-enabled", service)
	out, err := cmd.Output()
	output := strings.TrimSpace(string(out))

	if err != nil {
		if output == "disabled" {
			return false, nil
		}
		return false, fmt.Errorf("cannot query service state: %w", err)
	}

	return output == "enabled", nil
}

func EnableService(service string) error {
	cmd := exec.Command("systemctl", "--user", "enable", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot enable service: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func DisableService(service string) error {
	cmd := exec.Command("systemctl", "--user", "disable", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot disable service: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
