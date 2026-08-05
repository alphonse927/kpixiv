package wallpaper

import (
	"github.com/alphonse927/kpixiv/internal/logger"
)

// Screen identifies a currently active Plasma screen.
type Screen struct {
	ID      string // connector name, e.g. "DP-2" — stable across reboots, used as config key
	Index   string // plasma screen index, e.g. "0" — transient, used internally for wallpaper API
	Name    string
	Model   string
	Primary bool // true if this is the primary (main) display
}

// Label returns a human-readable display name for the screen, e.g. "DP-2 (Model)".
// It falls back to "Screen " + ID when no display name is known.
func (s Screen) Label() string {
	name := s.Name
	if name == "" {
		name = "Screen " + s.ID
	}

	if s.Model != "" {
		name = name + " (" + s.Model + ")"
	}

	return name
}

type Setter interface {
	Set(path string) error
}

// MonitorSetter is implemented by integrations that can target one screen.
type MonitorSetter interface {
	Setter
	Screens() ([]Screen, error)
	SetForScreen(screenID, path string) error
}

type DryRunSetter struct{}

// NewDryRunSetter creates a setter that only logs wallpaper changes.
func NewDryRunSetter() *DryRunSetter {
	return &DryRunSetter{}
}

// Set logs a wallpaper change without applying it.
func (d *DryRunSetter) Set(path string) error {
	log := logger.WithComponent("wallpaper")
	log.Info("Dry-run: skipping wallpaper apply", "path", path)
	return nil
}
