package cmd

import (
	"runtime/debug"
	"strings"
)

// Version is the CLI version, set by the release build with
// -ldflags "-X github.com/dcadolph/preen/v2/cmd.Version=<v>". When it is empty,
// version falls back to the module version go install records.
//
//nolint:gochecknoglobals // Build-time override target.
var Version = ""

// version returns the version to report.
func version() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(Version, info, ok)
}

// resolveVersion prefers the release build's version, then the module version
// from the build info, and reports dev for a local build that has neither.
func resolveVersion(set string, info *debug.BuildInfo, ok bool) string {
	if set != "" {
		return set
	}
	if ok && info != nil {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}
