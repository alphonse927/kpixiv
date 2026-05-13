package logger

import (
	"log/slog"
	"os"
)

var log *slog.Logger

func Init(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

func WithComponent(name string) *slog.Logger {
	return log.With("component", name)
}

func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

func Error(msg string, args ...any) {
	log.Error(msg, args...)
}
