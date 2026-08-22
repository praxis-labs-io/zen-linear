package main

import (
	"fmt"
	"runtime"
)

// Build metadata stamped at link time by .github/workflows/release.yml. A
// plain go build leaves the defaults, which is how a local binary is told
// apart from a released one.
var (
	// Version is the semantic version of the application.
	Version = "dev"
	// Commit is the git commit SHA of the build.
	Commit = "none"
	// Date is the build date.
	Date = "unknown"
)

// VersionInfo returns a formatted string with version details.
func VersionInfo() string {
	return fmt.Sprintf("zen-linear %s (commit: %s, built: %s, %s/%s)",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
}
