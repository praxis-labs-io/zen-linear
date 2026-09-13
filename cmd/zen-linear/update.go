package main

import (
	"context"
	"fmt"
	"io"

	"github.com/praxis-labs-io/zen-linear/internal/update"
)

func runUpdate(out, errOut io.Writer) int {
	dir, err := update.InstallDir()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
		return 1
	}

	ctx := context.Background()

	result, err := update.Check(ctx, update.Options{Current: Version})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Warning: could not check for a newer release: %v\n", err)
	}

	switch {
	case Version == update.DevVersion:
		_, _ = fmt.Fprintln(out, "This is a locally built binary. Installing the latest release over it.")
	case result.Available:
		_, _ = fmt.Fprintf(out, "%s is available, running %s.\n", result.Latest, Version)
	case result.Latest != "":
		_, _ = fmt.Fprintf(out, "%s is the latest release. Nothing to install.\n", result.Latest)
		return 0
	}

	if err := update.Install(ctx, update.InstallOptions{Dir: dir, Out: out}); err != nil {
		_, _ = fmt.Fprintf(errOut, "Update failed: %v\n", err)
		return 1
	}

	return 0
}
