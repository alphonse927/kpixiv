package build

import "runtime/debug"

func readBuildInfo() (*debug.BuildInfo, bool) {
	info, ok := debug.ReadBuildInfo()
	return info, ok
}
