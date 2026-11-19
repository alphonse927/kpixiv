// Package testutil holds helpers shared by the test binaries of this module.
package testutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/alphonse927/kpixiv/internal/logger"
)

// RunMain is meant to be called from a package's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RunMain(m)) }
//
// It points HOME at a throwaway directory for the whole test binary run and
// initializes the logger there. The logger writes its file under
// $HOME/.local/state, so without this a test run would append to the real
// user's kpixiv.log. It returns the exit code of m.Run.
func RunMain(m *testing.M) int {
	tmpHome, err := os.MkdirTemp("", "kpixiv-test-home-")
	if err != nil {
		panic(err)
	}

	if err = os.Setenv("HOME", tmpHome); err != nil {
		panic(err)
	}

	logger.Init(false)
	code := m.Run()
	logger.Close()

	if err = os.RemoveAll(tmpHome); err != nil {
		fmt.Fprintf(os.Stderr, "testutil: cannot remove %s: %v\n", tmpHome, err)
	}

	return code
}
