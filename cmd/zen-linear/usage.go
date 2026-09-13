package main

import (
	"io"
	"strings"
)

func printUsage(w io.Writer) {
	msg := strings.TrimSpace(`
zen-linear is a terminal client for Linear.

Usage:
  zen-linear               Open the app
  zen-linear auth login    Authenticate with Linear via browser OAuth
  zen-linear auth logout   Revoke and remove stored OAuth credentials
  zen-linear update        Install the latest release over this binary
  zen-linear help          Print this message
  zen-linear --version     Print the running version

LINEAR_API_KEY overrides stored OAuth credentials when set.
`) + "\n"
	_, _ = w.Write([]byte(msg))
}
