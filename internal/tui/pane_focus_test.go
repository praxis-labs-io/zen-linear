package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// stepKey drives the real global handler rather than stepPane, so a handler
// that claims h or l before the pane step shows up here.
func stepKey(app *App, r rune) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

func TestStepSkipsTheHiddenDetailsPane(t *testing.T) {
	app := newUXTestApp(t)
	if !app.detailsHidden {
		t.Fatal("details pane starts open; this test covers the closed case")
	}
	app.focusedPane = FocusIssues

	stepKey(app, 'l')

	if app.focusedPane != FocusIssues {
		t.Fatalf("l from Issues with details closed landed on %v, want to stay put", app.focusedPane)
	}

	stepKey(app, 'h')
	if app.focusedPane != FocusNavigation {
		t.Fatalf("h from Issues landed on %v, want Navigation", app.focusedPane)
	}
}

func TestStepSkipsTheHiddenNavigationPane(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.navigationHidden = true
	app.focusedPane = FocusIssues

	stepKey(app, 'l')
	if app.focusedPane != FocusDetails {
		t.Fatalf("l from Issues landed on %v, want Details", app.focusedPane)
	}
	stepKey(app, 'h')
	if app.focusedPane != FocusIssues {
		t.Fatalf("h from Details landed on %v, want Issues", app.focusedPane)
	}
}

// Tab belongs to a pane's own controls. It used to leave the pane too, which
// made those controls and pane navigation fight over one key.
func TestTabDoesNotMoveBetweenPanes(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsFocus = detailsFocusCards

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if app.focusedPane != FocusDetails {
		t.Fatalf("Tab from Details landed on %v, want to stay put", app.focusedPane)
	}
	if app.detailsFocus != detailsFocusCards {
		t.Fatal("Tab moved off the cards, where there is no box to walk")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone))
	if app.focusedPane != FocusDetails {
		t.Fatalf("Shift+Tab from Details landed on %v, want to stay put", app.focusedPane)
	}
}

// The details pane never dispatched command shortcuts, so every palette rune
// was dead there. < and > are how it showed up: the pane toggles did nothing.
func TestPaneTogglesFireFromTheDetailsPane(t *testing.T) {
	app := NewApp(linearapi.ClientConfig{}, config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	app.detailsHidden = false
	app.focusedPane = FocusDetails

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '<', tcell.ModNone))
	if !app.navigationHidden {
		t.Fatal("< did not hide the navigation pane from the details pane")
	}
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '<', tcell.ModNone))
	if app.navigationHidden {
		t.Fatal("< did not restore the navigation pane")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '>', tcell.ModNone))
	if !app.detailsHidden {
		t.Fatal("> did not hide the details pane from inside it")
	}
}

func TestDetailsPaneKeepsItsScrollKeys(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails

	for _, r := range []rune{'j', 'k', 'g', 'G'} {
		event := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
		if got := app.handleDetailsKey(event); got != event {
			t.Fatalf("%q was swallowed by the details pane instead of reaching the text view", r)
		}
	}
}

// TestTheDetailsPaneTitleIsJustDetails covers the tab strip being gone: the
// pane shows one thing and its border names one thing.
func TestTheDetailsPaneTitleIsJustDetails(t *testing.T) {
	app := newThreadedTestApp(t)

	title := stripTags(app.detailsView.GetTitle())
	if !strings.Contains(title, "Details") || !strings.Contains(title, "[3]") {
		t.Errorf("details title = %q, want it to name the pane and its number", title)
	}
	if strings.Contains(title, "Comments") {
		t.Errorf("details title = %q, want no tab strip", title)
	}
}
