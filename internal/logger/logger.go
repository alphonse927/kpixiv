package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alphonse927/kpixiv/internal/platform"
)

// maxLogFileBytes is the size at which the log file is rotated. Kept small
// on purpose: this is a desktop app's diagnostic log, not a service that
// needs long retention, and a small cap keeps "View Logs" fast to load.
const maxLogFileBytes = 5 * 1024 * 1024 // 5 MB

var (
	mu          sync.Mutex
	log         *slog.Logger
	writer      io.Writer = os.Stdout
	logFile     *os.File
	logFilePath string
)

// Init configures the global logger with info or debug level. It always
// writes to stdout and, best-effort, to the centralized log file at
// platform.LogFilePath() (~/.local/state/kpixiv/kpixiv.log). Writing to a
// fixed, known file -- rather than relying solely on stdout -- means the log
// viewer in Settings can show real logs regardless of whether kPixiv is
// running as the systemd service or via --foreground for debugging, instead
// of only working when running under systemd via journalctl.
func Init(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	openLogFile()
	setLogger(level)
}

// SetLevel reconfigures the global logger to the given level string.
// Supported values: debug, info, warn, error. Defaults to info if unrecognized.
// The underlying writer (stdout + log file, if available) is left as-is.
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

	setLogger(l)
}

func setLogger(level slog.Level) {
	mu.Lock()
	w := writer
	mu.Unlock()

	newLog := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}))

	mu.Lock()
	log = newLog
	mu.Unlock()
}

// openLogFile opens (creating if needed) the centralized log file, rotating
// it first if it has grown past maxLogFileBytes, and wires it into the
// package writer alongside stdout. Any failure here is non-fatal: the app
// keeps logging to stdout only, since a broken log file must never prevent
// kPixiv from starting.
func openLogFile() {
	path, err := platform.LogFilePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot determine log file path: %v\n", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot create log directory: %v\n", err)
		return
	}

	rotateIfOversized(path)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot open log file %s: %v\n", path, err)
		return
	}

	mu.Lock()
	if logFile != nil {
		_ = logFile.Close() //nolint:errcheck // best-effort close of a previous handle
	}
	logFile = f
	logFilePath = path
	writer = io.MultiWriter(os.Stdout, f)
	mu.Unlock()
}

// rotateIfOversized moves an existing oversized log file to a ".old"
// sibling before a fresh one is opened, keeping at most one prior rotation
// on disk rather than growing forever.
func rotateIfOversized(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxLogFileBytes {
		return
	}

	oldPath := path + ".old"
	if err := os.Rename(path, oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "logger: cannot rotate log file: %v\n", err)
	}
}

// FilePath returns the path of the centralized log file, or "" if file
// logging could not be initialized (in which case only stdout receives
// logs).
func FilePath() string {
	mu.Lock()
	defer mu.Unlock()
	return logFilePath
}

// Close flushes and closes the log file handle, if one is open. Safe to
// call even if Init was never called or file logging failed to initialize.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_ = logFile.Close() //nolint:errcheck // best-effort close on shutdown
		logFile = nil
	}
}

// WithComponent returns a logger tagged with a component name.
func WithComponent(name string) *slog.Logger {
	mu.Lock()
	l := log
	mu.Unlock()
	return l.With("component", name)
}

// Debug logs a debug-level message.
func Debug(msg string, args ...any) {
	mu.Lock()
	l := log
	mu.Unlock()
	l.Debug(msg, args...)
}

// Info logs an info-level message.
func Info(msg string, args ...any) {
	mu.Lock()
	l := log
	mu.Unlock()
	l.Info(msg, args...)
}

// Error logs an error-level message.
func Error(msg string, args ...any) {
	mu.Lock()
	l := log
	mu.Unlock()
	l.Error(msg, args...)
}
