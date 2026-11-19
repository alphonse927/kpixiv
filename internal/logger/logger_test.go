package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitDoesNotWriteOutsideTempDirInTests guards against test runs
// polluting a real log file: with HOME pointing outside the temp dir, Init
// must refuse to create or open any log file.
func TestInitDoesNotWriteOutsideTempDirInTests(t *testing.T) {
	fakeHome := filepath.Join(string(filepath.Separator), "kpixiv-test-nonexistent-home")
	t.Cleanup(func() { _ = os.RemoveAll(fakeHome) }) // best-effort cleanup if the guard ever fails
	t.Setenv("HOME", fakeHome)

	Init(false)
	t.Cleanup(Close)

	if got := FilePath(); strings.HasPrefix(got, fakeHome) {
		t.Fatalf("FilePath() = %q, want no log file under %q", got, fakeHome)
	}

	if _, err := os.Stat(fakeHome); !os.IsNotExist(err) {
		t.Fatalf("Init() created %q (stat error: %v), want it untouched", fakeHome, err)
	}
}

// TestInitWritesLogFileUnderTempHome checks that file logging still works
// when HOME is a temp dir, which is what the scheduler tests rely on.
func TestInitWritesLogFileUnderTempHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	Init(false)
	t.Cleanup(Close)

	path := FilePath()
	if !strings.HasPrefix(path, home) {
		t.Fatalf("FilePath() = %q, want it under %q", path, home)
	}

	Info("logger test message")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", path, err)
	}

	if !strings.Contains(string(data), "logger test message") {
		t.Errorf("log file does not contain the logged message: %q", data)
	}
}

func TestIsUnderTempDir(t *testing.T) {
	tmp := os.TempDir()

	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(tmp, "x", "kpixiv.log"), true},
		{filepath.Join(tmp, "..", "elsewhere", "kpixiv.log"), false},
		{filepath.Join(string(filepath.Separator), "home", "user", ".local", "state", "kpixiv", "kpixiv.log"), false},
	}

	for _, tc := range tests {
		if got := isUnderTempDir(tc.path); got != tc.want {
			t.Errorf("isUnderTempDir(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
