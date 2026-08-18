package platform

import (
	"os"
	"path/filepath"
)

const logFileName = "kpixiv.log"

// LogFilePath returns the on-disk location of kPixiv's log file:
// ~/.local/state/kpixiv/kpixiv.log. This is the single source of truth for
// the application's own logs, regardless of how the process was started --
// unlike the systemd journal, which only reflects a run's output while it
// was actually managed by systemd.
func LogFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "kpixiv", logFileName), nil
}
