package main

import (
	"fmt"
	"runtime"
)

// Stamped through ldflags by the release workflow; a local build keeps the defaults.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func VersionInfo() string {
	return fmt.Sprintf("zen-linear %s (commit: %s, built: %s, %s/%s)",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
}
