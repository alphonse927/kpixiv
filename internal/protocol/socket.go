package protocol

import (
	"os"
	"path/filepath"
	"strconv"
)

// DefaultSocketPath returns the runtime socket path used by the daemon and tray.
func DefaultSocketPath() string {
	if socketPath := os.Getenv("KPIXIV_SOCKET"); socketPath != "" {
		return socketPath
	}

	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "kpixiv.sock")
	}

	return filepath.Join(os.TempDir(), "kpixiv-"+strconv.Itoa(os.Getuid())+".sock")
}
