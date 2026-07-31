package build

import (
	"fmt"
	"runtime"
)

// Build information. Version, Commit, and Date are injected at build time via
// -ldflags (see the Makefile); the rest is derived at runtime.
var (
	// Version is the semantic version, derived from the git tag.
	Version = "dev"
	// Commit is the short git commit hash the binary was built from.
	Commit = ""
	// Date is the UTC build timestamp in RFC3339 format.
	Date = ""
	// GoVersion is the Go toolchain the binary was compiled with.
	GoVersion = runtime.Version()
)

// FyneVersion returns the Fyne module version embedded in the build info,
// or an empty string when it cannot be determined.
func FyneVersion() string {
	return moduleVersion("fyne.io/fyne/v2")
}

// CobraVersion returns the Cobra module version embedded in the build info,
// or an empty string when it cannot be determined.
func CobraVersion() string {
	return moduleVersion("github.com/spf13/cobra")
}

// Summary renders a compact, human-readable description of the build.
func Summary() string {
	line := fmt.Sprintf("kpixivctl %s (%s)", Version, GoVersion)
	if Commit != "" {
		line += fmt.Sprintf(", commit %s", Commit)
	}
	return line
}

func moduleVersion(path string) string {
	info, ok := readBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == path {
			return dep.Version
		}
	}
	return ""
}
