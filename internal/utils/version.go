package utils

import (
	"runtime/debug"
	"strings"
)

// version will be set by GoReleaser during builds
var version string

// GetVersion returns the current version of the application.
// If version is not set via ldflags, it will try to get it from Go's build info.
// If that fails, it returns "unknown".
// The version string will have any leading "v" prefix removed.
func GetVersion() string {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		} else {
			v = "unknown"
		}
	}
	return strings.TrimPrefix(v, "v")
}
