package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/update"
)

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
	if !strings.Contains(got, "zen-linear update") {
		t.Errorf("status bar = %q, want the upgrade command named", got)
	}
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

func TestANudgeTakesTheLineWhenNothingWentWrong(t *testing.T) {
	app := newUXTestApp(t)

	app.pendingNotice = updateNoticeText("v0.4.0")
	app.reportPendingNotice()

	if got := statusText(app); !strings.Contains(got, "v0.4.0") {
		t.Errorf("status bar = %q, want the notice shown", got)
	}
}

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

func TestTurningTheCheckOnAsksWithoutWaitingForTheNextLaunch(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = false
	app.version = "0.3.0"
	asked := make(chan string, 1)
	app.checkUpdateFunc = func(_ context.Context, version string) (update.Result, error) {
		asked <- version
		return update.Result{Latest: "v0.4.0", Available: true}, nil
	}

	cfg := app.config
	cfg.UpdateCheck = true
	app.applySettings(cfg)

	select {
	case got := <-asked:
		if got != "0.3.0" {
			t.Errorf("checked version = %q, want the running one", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("turning the check on did not ask until the next launch")
	}
}

func TestASaveWithTheCheckAlreadyOnDoesNotAskAgain(t *testing.T) {
	app := newUXTestApp(t)
	app.config.UpdateCheck = true
	app.version = "0.3.0"
	asked := make(chan struct{}, 1)
	app.checkUpdateFunc = func(context.Context, string) (update.Result, error) {
		asked <- struct{}{}
		return update.Result{Latest: "v0.4.0", Available: true}, nil
	}

	cfg := app.config
	cfg.Theme = config.ThemeLinear
	app.applySettings(cfg)
	settle()

	select {
	case <-asked:
		t.Error("the check ran for a save that did not turn it on")
	default:
	}
}
