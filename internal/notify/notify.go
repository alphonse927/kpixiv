package notify

import (
	"os/exec"
	"sync"

	"github.com/alphonse927/kpixiv/internal/logger"
)

// Notifier posts desktop notifications through notify-send. When no
// notification daemon is available (headless systems, non-KDE desktops,
// missing notify-send), notifications degrade gracefully to log lines.
type Notifier struct {
	mu     sync.Mutex
	binary string // cached path to notify-send, "" when unavailable
	probed bool
}

var defaultNotifier = &Notifier{}

// Send posts a notification. The message is always logged; it is only shown
// on the desktop when notifications are enabled and a daemon is available.
func (n *Notifier) Send(title, body string) {
	if !Enabled() {
		logger.Debug("Skipping desktop notification (disabled)", "title", title, "body", body)
		return
	}

	binary := n.findBinary()
	if binary == "" {
		logger.Debug("Skipping desktop notification (notify-send unavailable)", "title", title, "body", body)
		return
	}

	// #nosec G204 -- arguments are application-controlled message strings.
	if err := exec.Command(binary, "-a", "kpixiv", "-i", "kpixiv", title, body).Start(); err != nil {
		logger.Debug("Failed to send desktop notification", "title", title, "error", err)
	}

	logger.Debug("Desktop notification sent", "title", title, "body", body)
}

// SendDefault posts a notification using the package-level notifier.
func SendDefault(title, body string) {
	defaultNotifier.Send(title, body)
}

func (n *Notifier) findBinary() string {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.probed {
		return n.binary
	}
	n.probed = true

	for _, name := range []string{"notify-send", "notify-send-ng"} {
		if path, err := exec.LookPath(name); err == nil {
			n.binary = path
			return n.binary
		}
	}

	return ""
}
