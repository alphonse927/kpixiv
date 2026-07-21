package wallpaper

import (
	"github.com/alphonse927/kpixiv/internal/logger"
)

// Screen identifies a currently active Plasma screen.
type Screen struct {
	ID    string
	Name  string
	Model string
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
