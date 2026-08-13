package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// TestSearchOpensFromEveryPane covers the one key that has to work from
// anywhere. The nav pane can be toggled off or dropped by a narrow layout, and
// a query box nobody can reach is a query box nobody has.
func TestSearchOpensFromEveryPane(t *testing.T) {
	for _, pane := range []FocusTarget{FocusNavigation, FocusIssues, FocusDetails} {
		app := newUXTestApp(t)
		app.focusedPane = pane
		app.navigationHidden = true

		if got := app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone)); got != nil {
			t.Fatalf("from %v, / leaked past the global keys", pane)
		}
		if app.navigationHidden {
			t.Errorf("from %v, / left the nav pane hidden", pane)
		}
		if !app.navSearchActive() {
			t.Errorf("from %v, / did not put the keyboard in the query box", pane)
		}
	}
}

// TestTheQueryBoxSwallowsGlobalRunes is why navSearchActive gates above the
// global rune switch. Without it, typing a word with a q in it quits.
func TestTheQueryBoxSwallowsGlobalRunes(t *testing.T) {
	app := newUXTestApp(t)
	app.focusNavSearch()

	for _, r := range []rune{'q', ':', '/', '1', '2', '3', 'r'} {
		event := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
		if got := app.handleGlobalKey(event); got != event {
			t.Errorf("%q did not reach the field", r)
		}
		if !app.navSearchActive() {
			t.Fatalf("%q fired a shortcut and moved the keyboard out of the box", r)
		}
	}
}

// TestDownAndTabReachTheTree covers the pane's own ring: two controls under one
// border, and both keys walk between them.
func TestDownAndTabReachTheTree(t *testing.T) {
	for _, key := range []tcell.Key{tcell.KeyDown, tcell.KeyTab} {
		app := newUXTestApp(t)
		app.focusNavSearch()

		if got := app.handleGlobalKey(tcell.NewEventKey(key, 0, tcell.ModNone)); got != nil {
			t.Fatalf("%v leaked past the query box", key)
		}
		if app.navSearchFocused {
			t.Errorf("%v left the keyboard in the query box", key)
		}
		if app.app.GetFocus() != tview.Primitive(app.navigationTree) {
			t.Errorf("%v did not land on the tree", key)
		}

		// Tab is a ring, so it comes back.
		app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
		if !app.navSearchFocused {
			t.Errorf("after %v, Tab did not return to the query box", key)
		}
	}
}

// TestEscClearsThenLetsGo covers both stops. Esc on a live query is a clear,
// not an exit: leaving with the words still there strands results nobody asked
// to keep.
func TestEscClearsThenLetsGo(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"})
	app.focusNavSearch()
	app.navSearchInput.SetText("found")
	app.performIssueSearch("found")
	waitForResults()

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.navSearchInput.GetText() != "" {
		t.Errorf("Esc left %q in the box", app.navSearchInput.GetText())
	}
	if app.activeIssuesSection != IssuesSectionList {
		t.Errorf("Esc left the pane on %v, want the list back", app.activeIssuesSection)
	}
	if !app.navSearchFocused {
		t.Error("the first Esc let go of the box as well as clearing it")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.navSearchFocused {
		t.Error("Esc on an empty box did not hand the keyboard to the tree")
	}
}

// TestPickingANavigationNodeDropsTheSearch covers the collision: the results
// are holding the pane the picked list wants.
func TestPickingANavigationNodeDropsTheSearch(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"})
	app.focusNavSearch()
	app.navSearchInput.SetText("found")
	app.performIssueSearch("found")
	waitForResults()

	app.onNavigationSelected(&NavigationNode{ID: "team-1", Text: "Zen Linear", TeamID: "team-1", IsTeam: true})

	if app.navSearchInput.GetText() != "" {
		t.Errorf("the query survived a navigation pick: %q", app.navSearchInput.GetText())
	}
	if app.activeIssuesSection != IssuesSectionList {
		t.Errorf("the pane stayed on %v after a navigation pick", app.activeIssuesSection)
	}
	if len(app.searchIssueRows) != 0 {
		t.Errorf("search rows = %d, want the results dropped", len(app.searchIssueRows))
	}
}

// TestThemeChangeRebuildsTheQueryBoxWithoutResearching covers two traps at once:
// tview bakes InputBg at construction so the box has to be rebuilt, and a
// rebuild that installs the change handler before the text re-fires the search.
func TestThemeChangeRebuildsTheQueryBoxWithoutResearching(t *testing.T) {
	app := newUXTestApp(t)
	app.navSearchInput.SetText("found")

	// A re-fired search shows up as a scheduled debounce, which is what the
	// change handler does and the only thing it does.
	scheduled := app.searchDebounceGeneration.Load()
	previous := app.navSearchInput
	app.theme = HighContrastTheme
	app.applyThemeToComponents()

	if app.navSearchInput == previous {
		t.Error("the query box was restyled rather than rebuilt, so InputBg is stale")
	}
	if got := app.navSearchInput.GetText(); got != "found" {
		t.Errorf("the query did not survive the rebuild: %q", got)
	}
	if got := app.searchDebounceGeneration.Load(); got != scheduled {
		t.Error("the rebuild re-fired the search")
	}
	if app.contentFlex.GetItem(0) != tview.Primitive(app.navigationPanel) {
		t.Error("the layout still holds the old panel, so the screen keeps a stale primitive")
	}
}
