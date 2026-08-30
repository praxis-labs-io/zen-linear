package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/update"
)

// waitForNotice gives the check's goroutine and its queued draw a moment to
// land. The seam answers immediately, so this is a handoff rather than a wait
// on anything real.
func waitForNotice(t *testing.T, app *App, want string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		got = statusText(app)
		if strings.Contains(got, want) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	return got
}

// settle lets a check that should do nothing prove it, since there is no state
// change to wait on.
func settle() { time.Sleep(100 * time.Millisecond) }

func TestAnAvailableReleaseIsOfferedOnTheHintLine(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = true
	app.version = "0.3.0"
	app.checkUpdateFunc = func(context.Context, string) (update.Result, error) {
		return update.Result{Latest: "v0.4.0", Available: true}, nil
	}

	app.startUpdateCheck()

	got := waitForNotice(t, app, "v0.4.0")
	if !strings.Contains(got, "v0.4.0") {
		t.Errorf("status bar = %q, want the available version named", got)
	}
	// The number alone leaves the reader to work out how to act on it.
	if !strings.Contains(got, "installer") {
		t.Errorf("status bar = %q, want the upgrade path named", got)
	}
	// A release being available is not a failure and must not be dressed as one.
	if strings.Contains(got, "Error:") {
		t.Errorf("status bar = %q, want no error prefix", got)
	}
}

func TestTheRunningVersionIsOfferedNothing(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = true
	app.version = "0.3.0"
	app.updateStatusBar()
	before := statusText(app)
	app.checkUpdateFunc = func(context.Context, string) (update.Result, error) {
		return update.Result{Latest: "v0.3.0", Available: false}, nil
	}

	app.startUpdateCheck()
	settle()

	if got := statusText(app); got != before {
		t.Errorf("status bar = %q, want the hints left alone at %q", got, before)
	}
}

// No network, a rate limit, a malformed answer: the log carries it and the user
// is told nothing.
func TestAFailedCheckSaysNothing(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = true
	app.version = "0.3.0"
	app.updateStatusBar()
	before := statusText(app)
	app.checkUpdateFunc = func(context.Context, string) (update.Result, error) {
		return update.Result{}, errors.New("the latest release lookup answered 403 Forbidden")
	}

	app.startUpdateCheck()
	settle()

	if got := statusText(app); got != before {
		t.Errorf("status bar = %q, want the hints left alone at %q", got, before)
	}
}

func TestTheCheckIsNotRunWhenTurnedOff(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = false
	app.version = "0.3.0"
	asked := make(chan struct{}, 1)
	app.checkUpdateFunc = func(context.Context, string) (update.Result, error) {
		asked <- struct{}{}
		return update.Result{Latest: "v0.4.0", Available: true}, nil
	}

	app.startUpdateCheck()
	settle()

	select {
	case <-asked:
		t.Error("the check ran with the setting off")
	default:
	}
}

// An unstamped build is a working tree, which is not behind anything.
func TestAnUnstampedBuildIsNeverChecked(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = true
	app.version = ""
	asked := make(chan struct{}, 1)
	app.checkUpdateFunc = func(context.Context, string) (update.Result, error) {
		asked <- struct{}{}
		return update.Result{Latest: "v0.4.0", Available: true}, nil
	}

	app.startUpdateCheck()
	settle()

	select {
	case <-asked:
		t.Error("a build with no stamped version was checked")
	default:
	}
}

// A log the app could not open is a problem. This is a nudge, and it keeps
// until the next launch rather than painting over one.
func TestALaunchWarningKeepsTheLineAheadOfANudge(t *testing.T) {
	app := newUXTestApp(t)
	app.WarnAtStartup("could not open the log at /nope/app.log")
	app.reportPendingWarning()
	warned := statusText(app)

	app.pendingNotice = updateNoticeText("v0.4.0")
	app.reportPendingNotice()

	if got := statusText(app); got != warned {
		t.Errorf("status bar = %q, want the warning kept at %q", got, warned)
	}
}

// Without a warning to defer to, the nudge takes the line.
func TestANudgeTakesTheLineWhenNothingWentWrong(t *testing.T) {
	app := newUXTestApp(t)

	app.pendingNotice = updateNoticeText("v0.4.0")
	app.reportPendingNotice()

	if got := statusText(app); !strings.Contains(got, "v0.4.0") {
		t.Errorf("status bar = %q, want the notice shown", got)
	}
}

// Reported once. A later refresh that re-reports must not put it back, the way
// a launch warning is forgotten after it is shown.
func TestANudgeIsShownOnce(t *testing.T) {
	app := newUXTestApp(t)
	app.pendingNotice = updateNoticeText("v0.4.0")
	app.reportPendingNotice()

	app.updateStatusBar()
	app.reportPendingNotice()

	if got := statusText(app); strings.Contains(got, "v0.4.0") {
		t.Errorf("status bar = %q, want the notice not repeated", got)
	}
}
