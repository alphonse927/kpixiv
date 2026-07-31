package notify

import "sync"

// enabled guards the package-level notification toggle. It defaults to on;
// the application sets it from the configuration at startup and whenever the
// configuration changes.
var (
	enabledMu sync.RWMutex
	enabled   = true
)

// SetEnabled controls whether SendDefault delivers desktop notifications.
// Disabling does not stop messages from being written to the log.
func SetEnabled(on bool) {
	enabledMu.Lock()
	enabled = on
	enabledMu.Unlock()
}

// Enabled reports whether desktop notifications are currently delivered.
func Enabled() bool {
	enabledMu.RLock()
	defer enabledMu.RUnlock()
	return enabled
}
