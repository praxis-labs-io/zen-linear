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

// TestDelegatedFocusReachesTheTree covers the pane going dead on launch.
// SetRoot hands focus down the primitive tree, and a Flex with no item flagged
// keeps it on its own Box, which answers no keys: the border says the pane is
// focused and the arrows do nothing.
func TestDelegatedFocusReachesTheTree(t *testing.T) {
	app := newUXTestApp(t)

	app.app.SetFocus(app.pages)

	if got := app.app.GetFocus(); got != tview.Primitive(app.navigationTree) {
		t.Fatalf("delegated focus landed on %T, want the navigation tree", got)
	}
}

// TestClickingEitherControlMovesTheKeyboardWithIt covers the mouse skipping
// updateFocus entirely. A click that focuses the widget without saying so
// leaves handleGlobalKey routing to the pane the user clicked out of, and in
// the query box that means q quits with the caret sitting in it.
func TestClickingEitherControlMovesTheKeyboardWithIt(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	// The pane the click has to take the keyboard away from.
	app.focusedPane = FocusIssues

	clickNavPane(t, app, app.navSearchInput)
	if !app.navSearchActive() {
		t.Fatal("clicking the query box left the keys with the pane the user clicked out of")
	}
	event := tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone)
	if got := app.handleGlobalKey(event); got != event {
		t.Fatal("q fired the quit shortcut with the caret in the query box")
	}

	clickNavPane(t, app, app.navigationTree)
	if app.navSearchActive() {
		t.Error("clicking the tree left the query box holding the keys")
	}
	if app.focusedPane != FocusNavigation {
		t.Errorf("clicking the tree left the pane on %v", app.focusedPane)
	}
}

// clickNavPane focuses a primitive the way a mouse click does: straight through
// tview, with none of the app's own focus bookkeeping.
func clickNavPane(t *testing.T, app *App, target tview.Primitive) {
	t.Helper()
	app.app.SetFocus(target)
}

// TestAnOverlayKeepsTheKeysWhileItsPageChurns covers what the nav pane's focus
// claim must not do. tview re-delegates focus down the whole tree on every page
// add and remove, and that walk reaches this pane. The palette rebuilds its
// page on each keystroke, so a claim there took the pane back mid-rebuild and
// the palette's own re-show guard then failed silently: every key closed it.
func TestAnOverlayKeepsTheKeysWhileItsPageChurns(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.app.SetRoot(app.pages, true)
	app.openPalette()

	for _, event := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone),
	} {
		app.handleGlobalKey(event)
		if front, _ := app.pages.GetFrontPage(); front != "palette" {
			t.Fatalf("%v closed the palette; the front page is %q", event.Key(), front)
		}
		if app.focusedPane != FocusPalette {
			t.Fatalf("%v handed the pane to %v while the palette was up", event.Key(), app.focusedPane)
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

// TestUpOffTheTopOfTheTreeReachesTheQueryBox covers the return trip for the
// Down that left it. Anywhere else in the tree, Up is the tree's own move.
func TestUpOffTheTopOfTheTreeReachesTheQueryBox(t *testing.T) {
	for _, key := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone),
	} {
		app := newUXTestApp(t)
		app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
		rows := app.navigationTree.GetRoot().GetChildren()
		if len(rows) < 2 {
			t.Fatalf("fixture has %d tree rows, want at least two", len(rows))
		}

		app.navigationTree.SetCurrentNode(rows[1])
		if got := app.handleGlobalKey(key); got == nil {
			t.Errorf("%v was claimed mid-tree, want it left to the tree", key.Key())
		}

		app.navigationTree.SetCurrentNode(rows[0])
		if got := app.handleGlobalKey(key); got != nil {
			t.Errorf("%v leaked past the top of the tree", key.Key())
		}
		if !app.navSearchActive() {
			t.Errorf("%v off the top of the tree did not reach the query box", key.Key())
		}
	}
}

// TestSearchResultsPutOutTheTreeSelection covers a lit row claiming to name the
// list on screen while what is on screen is a workspace-wide search, which
// takes no list. Stepping into the tree relights it: an unlit cursor is one
// nobody can steer.
func TestSearchResultsPutOutTheTreeSelection(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me"})
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	current := app.navigationTree.GetRoot().GetChildren()[0]
	app.navigationTree.SetCurrentNode(current)
	app.focusNavSearch()

	lit := selectionStyle(app.theme)
	if got := current.GetSelectedTextStyle(); got != lit {
		t.Fatalf("the tree row is not lit to start with: %v", got)
	}

	app.performIssueSearch("found")
	waitForResults()
	if got := current.GetSelectedTextStyle(); got == lit {
		t.Error("the tree row stayed lit while search results held the pane")
	}

	app.focusNavigationTree()
	if got := current.GetSelectedTextStyle(); got != lit {
		t.Error("the tree took the keyboard with its cursor still out, so nothing on screen says where the arrows are")
	}

	app.focusNavSearch()
	app.performIssueSearch("")
	if got := current.GetSelectedTextStyle(); got != lit {
		t.Error("the tree row stayed out after the list came back")
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
