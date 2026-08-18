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

	if err = os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("cannot create systemd user directory: %w", err)
	}

	if err = os.WriteFile(path, []byte(serviceUnit), 0600); err != nil {
		return fmt.Errorf("cannot write service unit: %w", err)
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

// IsServiceActive reports whether the given systemd user service is currently
// active. It returns false when systemd reports an unknown/not-found state.
func IsServiceActive(service string) bool {
	//nolint:gosec // service name is controlled by the application, not user input
	out, err := exec.Command("systemctl", "--user", "is-active", service).Output()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// SystemdAvailable reports whether systemctl is available on this system.
// kPixiv only supports running as a systemd user service; when systemctl
// isn't present (e.g. a non-systemd distro), callers should fall back to
// running in the foreground with a clear warning rather than silently
// pretending background supervision exists.
func SystemdAvailable() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// StartService ensures the unit is installed and running right now. Unlike
// EnableService, it does not affect whether the service starts on the next
// login -- it only makes sure it's running in the current session. This is
// what a manual `kpixiv` launch uses to hand off to the systemd-managed
// instance instead of becoming a second, competing process.
func StartService(service string) error {
	if err := InstallServiceUnit(); err != nil {
		return err
	}

	//nolint:gosec // service name is controlled by the application, not user input
	cmd := exec.Command("systemctl", "--user", "start", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot start service: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func EnableService(service string) error {
	if err := InstallServiceUnit(); err != nil {
		return err
	}

	// --now also starts the service immediately, not just on the next
	// login. Without it, checking "start automatically" in Settings had no
	// visible effect until the next session, so people kept launching
	// kPixiv by hand every time -- which is exactly the parallel,
	// unsupervised execution path this project no longer supports.
	//nolint:gosec // service name is controlled by the application, not user input
	cmd := exec.Command("systemctl", "--user", "enable", "--now", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot enable service: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

func DisableService(service string) error {
	// --now also stops the service immediately. If this call is made from
	// within the running GUI itself (i.e. this process IS the systemd
	// service), it will receive SIGTERM and shut down gracefully right
	// after this returns -- that's expected: turning off "run in the
	// background" should mean kPixiv actually stops, not just "won't
	// restart next time."
	//nolint:gosec // service name is controlled by the application, not user input
	cmd := exec.Command("systemctl", "--user", "disable", "--now", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot disable service: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}
