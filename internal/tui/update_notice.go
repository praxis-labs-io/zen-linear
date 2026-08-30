package tui

import (
	"context"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/update"
)

// checkForUpdate is the seam the launch runs. Tests replace it rather than
// reaching GitHub, the way the fetch funcs around it are replaced.
type checkForUpdate func(ctx context.Context, current string) (update.Result, error)

// fetchLatestRelease asks GitHub, caching the answer for a day. The path is
// resolved here rather than passed down from main: nothing about the launch
// waits on it, and a home directory that will not resolve is a check that does
// not run rather than a launch that fails.
func fetchLatestRelease(ctx context.Context, current string) (update.Result, error) {
	path, err := update.Path()
	if err != nil {
		return update.Result{}, fmt.Errorf("resolve the update cache path: %w", err)
	}
	return update.Check(ctx, update.Options{Current: current, CachePath: path})
}

// updateChecker returns the seam to run, defaulting to the real one so only a
// test has to set it.
func (a *App) updateChecker() checkForUpdate {
	if a.checkUpdateFunc != nil {
		return a.checkUpdateFunc
	}
	return fetchLatestRelease
}

// startUpdateCheck asks whether a newer release exists, off the UI thread and
// off the launch's critical path: nothing waits on it, and it holds the answer
// for the status bar rather than blocking anything that draws.
//
// The check is silent about its own failures. No network, a rate limit, a
// malformed answer: the log carries it and the user is told nothing, because a
// nudge that reports why it could not nudge is worse than one that stays quiet.
func (a *App) startUpdateCheck() {
	if !a.config.UpdateCheck {
		return
	}
	// Snapshotted on the UI thread, like every other seam the launch hands to a
	// goroutine.
	current := a.version
	check := a.updateChecker()
	if current == "" {
		return
	}

	go func() {
		result, err := check(context.Background(), current)
		if err != nil {
			logger.Debug("tui.update: update check did not complete: %v", err)
			return
		}
		if !result.Available {
			return
		}
		logger.Info("tui.update: %s is available, running %s", result.Latest, current)
		a.QueueUpdateDraw(func() {
			a.pendingNotice = updateNoticeText(result.Latest)
			a.reportPendingNotice()
		})
	}()
}

// updateNoticeText names the upgrade rather than only the version, since the
// number alone leaves the reader to go and find out how.
func updateNoticeText(latest string) string {
	return fmt.Sprintf("%s is available. Re-run the installer to upgrade.", latest)
}
