// Package testutil holds helpers shared by the test binaries of this module.
// This package must stay free of imports that depend on the packages whose
// tests call RunMain (pixiv, fetcher, scheduler, app), or those tests would
// form an import cycle. Fakes that need such packages live in sub-packages
// (see pixivtest).
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphonse927/kpixiv/internal/logger"
	"github.com/alphonse927/kpixiv/internal/notify"
	"github.com/alphonse927/kpixiv/internal/storage"
)

// RunMain is meant to be called from a package's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RunMain(m)) }
//
// It points HOME at a throwaway directory for the whole test binary run and
// initializes the logger there. The logger writes its file under
// $HOME/.local/state, so without this a test run would append to the real
// user's kpixiv.log. It also turns desktop notifications off, so tests that
// reach the scheduler's or the controller's notify calls do not pop up real
// notifications. It returns the exit code of m.Run.
func RunMain(m *testing.M) int {
	tmpHome, err := os.MkdirTemp("", "kpixiv-test-home-")
	if err != nil {
		panic(err)
	}

	if err = os.Setenv("HOME", tmpHome); err != nil {
		panic(err)
	}

	logger.Init(false)
	notify.SetEnabled(false)
	code := m.Run()
	logger.Close()

	if err = os.RemoveAll(tmpHome); err != nil {
		fmt.Fprintf(os.Stderr, "testutil: cannot remove %s: %v\n", tmpHome, err)
	}

	return code
}

// NewStorage returns a Storage rooted in a fresh temp directory that is
// removed when the test ends. Downloads go to a "downloads" folder inside it.
func NewStorage(t testing.TB) *storage.Storage {
	t.Helper()
	tmp := t.TempDir()

	st, err := storage.New(tmp, filepath.Join(tmp, "downloads"))
	if err != nil {
		t.Fatalf("storage.New() returned error: %v", err)
	}

	return st
}
