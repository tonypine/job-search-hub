// Package buildinfo is the hub's version, stamped into every Go command at
// build time with scripts/release/version.sh --go-ldflags.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// version and commit are set with -ldflags -X; a build without them falls
// back to the commit Go recorded, as a dev version.
var (
	version string
	commit  string
)

// devMajorMinor names a build no release script stamped, as version.sh does.
const devMajorMinor = "0.1"

// Version is the hub's version: 0.1.<N> for a release, where N counts the
// commits on main, and 0.1.0-dev.<short commit> for any other build.
func Version() string {
	if version != "" {
		return version
	}
	if short := shortCommit(Commit()); short != "" {
		return devMajorMinor + ".0-dev." + short
	}
	return devMajorMinor + ".0-dev"
}

// Commit is the commit the build is made from, or "unknown".
func Commit() string {
	if commit != "" {
		return commit
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}
	return "unknown"
}

// IsRelease says whether the version is a release's, not a dev build's.
func IsRelease() bool {
	return !strings.Contains(Version(), "-dev")
}

func shortCommit(commit string) string {
	if commit == "unknown" {
		return ""
	}
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
