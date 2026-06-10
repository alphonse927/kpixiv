package tray

import "embed"

//go:embed assets/*
var assets embed.FS

func loadIconPNG() []byte {
	data, err := assets.ReadFile("assets/kpixiv.png")
	if err != nil {
		return nil
	}

	return data
}
