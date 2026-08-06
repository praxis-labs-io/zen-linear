package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// tabKey drives the real global handler rather than cyclePanes, so an
// interception layer between the key and the cycle shows up here.
func tabKey(app *App, backward bool) {
	key := tcell.KeyTab
	if backward {
		key = tcell.KeyBacktab
	}
	app.handleGlobalKey(tcell.NewEventKey(key, 0, tcell.ModNone))
}

func TestTabSkipsTheHiddenDetailsPane(t *testing.T) {
	app := newUXTestApp(t)
	if !app.detailsHidden {
		t.Fatal("details pane starts open; this test covers the closed case")
	}
	app.focusedPane = FocusIssues

	tabKey(app, false)

	if app.focusedPane != FocusNavigation {
		t.Fatalf("Tab from Issues with details closed landed on %v, want Navigation", app.focusedPane)
	}

	tabKey(app, false)
	if app.focusedPane != FocusIssues {
		t.Fatalf("Tab from Navigation landed on %v, want Issues", app.focusedPane)
	}
}

func TestTabSkipsTheHiddenNavigationPane(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.navigationHidden = true
	app.focusedPane = FocusIssues

	tabKey(app, false)
	if app.focusedPane != FocusDetails {
		t.Fatalf("Tab from Issues landed on %v, want Details", app.focusedPane)
	}
	tabKey(app, false)
	if app.focusedPane != FocusIssues {
		t.Fatalf("Tab wrapping past a hidden Navigation landed on %v, want Issues", app.focusedPane)
	}
}

// Tab is pane navigation. Details and Comments are a tab strip inside one pane,
// and Tab used to walk them, which made the pane cycle take two presses to leave.
func TestTabLeavesTheDetailsPaneWithoutWalkingItsTabs(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.detailsCommentsVisible = true
	app.focusedPane = FocusDetails
	app.focusedDetailsView = false

	tabKey(app, false)

	if app.focusedPane != FocusNavigation {
		t.Fatalf("Tab from Details landed on %v, want Navigation", app.focusedPane)
	}
	if app.focusedDetailsView {
		t.Fatal("Tab moved the Details/Comments tab on its way out of the pane")
	}
}

func TestShiftTabLeavesTheDetailsPaneWithoutWalkingItsTabs(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.detailsCommentsVisible = true
	app.focusedPane = FocusDetails
	app.focusedDetailsView = true

	tabKey(app, true)

	if app.focusedPane != FocusIssues {
		t.Fatalf("Shift+Tab from Details landed on %v, want Issues", app.focusedPane)
	}
	if !app.focusedDetailsView {
		t.Fatal("Shift+Tab moved the Details/Comments tab on its way out of the pane")
	}
}

// The details pane never dispatched command shortcuts, so every palette rune
// was dead there. { and } are how it showed up: the pane toggles did nothing.
// Both toggles take their rune from config, so the binding is part of the setup.
func TestPaneTogglesFireFromTheDetailsPane(t *testing.T) {
	app := NewApp(&linearapi.Client{}, config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
		Keybindings: map[string]string{
			// The tab keys own { and } by default, so reaching the toggles on
			// those runes means moving the tab keys off them first.
			"tab_prev":               "[",
			"tab_next":               "]",
			"toggle_navigation_pane": "{",
			"toggle_details_pane":    "}",
		},
	}, nil)
	stopDetailTimersOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	app.detailsHidden = false
	app.focusedPane = FocusDetails

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '{', tcell.ModNone))
	if !app.navigationHidden {
		t.Fatal("{ did not hide the navigation pane from the details pane")
	}
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '{', tcell.ModNone))
	if app.navigationHidden {
		t.Fatal("{ did not restore the navigation pane")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '}', tcell.ModNone))
	if !app.detailsHidden {
		t.Fatal("} did not hide the details pane from inside it")
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

func TestDetailsTabKeysStillCycleItsTabs(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.detailsCommentsVisible = true
	app.focusedPane = FocusDetails
	app.focusedDetailsView = false

	app.handleDetailsKey(tcell.NewEventKey(tcell.KeyRune, app.actionKey("tab_next", '}'), tcell.ModNone))
	if !app.focusedDetailsView {
		t.Fatal("the details tab key did not move to Comments")
	}
	if !strings.Contains(app.detailsTabsTitle(true), "Comments") {
		t.Fatalf("details strip %q missing the Comments tab", app.detailsTabsTitle(true))
	}
}
