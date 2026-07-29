package platform

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const socketName = "kpixiv.sock"

func instanceSocketPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "kpixiv", socketName), nil
}

// TryAcquire attempts to claim the single-instance socket.
// If another instance is already running, it returns an error
// and the caller should exit silently.
// If successful, it returns a listener that must be closed on shutdown.
func TryAcquire() (net.Listener, error) {
	path, err := instanceSocketPath()
	if err != nil {
		return nil, err
	}

	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("cannot create state directory: %w", err)
	}

	// If the socket exists and we can connect, another instance is running
	if conn, dialErr := net.Dial("unix", path); dialErr == nil {
		conn.Close()
		return nil, fmt.Errorf("another instance is already running")
	}

	// Remove stale socket and take over
	os.Remove(path)

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on socket: %w", err)
	}

	return listener, nil
}
