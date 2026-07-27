package main

import (
	"context"

	"github.com/alphonse927/kpixiv/internal/protocol"
	"github.com/alphonse927/kpixiv/internal/tray"
)

func main() {
	tray.Run(context.Background(), protocol.NewClient(protocol.DefaultSocketPath()))
}
