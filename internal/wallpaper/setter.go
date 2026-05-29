package wallpaper

import (
	"github.com/alphonse927/kpixiv/internal/logger"
)

type Setter interface {
	Set(path string) error
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
