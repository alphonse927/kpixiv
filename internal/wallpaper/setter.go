package wallpaper

import (
	"github.com/alphonse927/kpixiv/internal/logger"
)

type Setter interface {
	Set(path string) error
}

type DryRunSetter struct{}

func NewDryRunSetter() *DryRunSetter {
	return &DryRunSetter{}
}

func (d *DryRunSetter) Set(path string) error {
	log := logger.WithComponent("wallpaper")
	log.Info("Dry-run: skipping wallpaper apply", "path", path)
	return nil
}
