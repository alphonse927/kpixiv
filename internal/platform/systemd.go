package platform

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed kpixiv.service
var serviceUnit string

const serviceName = "kpixiv.service"

func serviceUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	return filepath.Join(home, ".config", "systemd", "user", serviceName), nil
}

func InstallServiceUnit() error {
	path, err := serviceUnitPath()
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("cannot create systemd user directory: %w", err)
	}

	if err = os.WriteFile(path, []byte(serviceUnit), 0644); err != nil {
		return fmt.Errorf("cannot write service unit: %w", err)
	}

	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("cannot reload systemd: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func RemoveServiceUnit() error {
	path, err := serviceUnitPath()
	if err != nil {
		return err
	}

	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove service unit: %w", err)
	}

	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("cannot reload systemd: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func IsServiceEnabled(service string) (bool, error) {
	path, err := serviceUnitPath()
	if err != nil {
		return false, err
	}

	if _, err = os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}

	//nolint:gosec // service name is controlled by the application, not user input
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
	if err := InstallServiceUnit(); err != nil {
		return err
	}

	//nolint:gosec // service name is controlled by the application, not user input
	cmd := exec.Command("systemctl", "--user", "enable", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot enable service: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func DisableService(service string) error {
	//nolint:gosec // service name is controlled by the application, not user input
	cmd := exec.Command("systemctl", "--user", "disable", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot disable service: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}
