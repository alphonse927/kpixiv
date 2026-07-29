package logger

import (
	"log/slog"
	"os"
	"strings"
)

var log *slog.Logger

// Init configures the global logger with info or debug level.
func Init(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

// SetLevel reconfigures the global logger to the given level string.
// Supported values: debug, info, warn, error. Defaults to info if unrecognized.
func SetLevel(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: l,
	}))
}

// WithComponent returns a logger tagged with a component name.
func WithComponent(name string) *slog.Logger {
	return log.With("component", name)
}

// Debug logs a debug-level message.
func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

// Info logs an info-level message.
func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

// Error logs an error-level message.
func Error(msg string, args ...any) {
	log.Error(msg, args...)
}
