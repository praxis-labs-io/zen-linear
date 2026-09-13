package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

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

func TestDelegatedFocusReachesTheTree(t *testing.T) {
	app := newUXTestApp(t)

	app.app.SetFocus(app.pages)

	if got := app.app.GetFocus(); got != tview.Primitive(app.navigationTree) {
		t.Fatalf("delegated focus landed on %T, want the navigation tree", got)
	}
}

func TestClickingEitherControlMovesTheKeyboardWithIt(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
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

func clickNavPane(t *testing.T, app *App, target tview.Primitive) {
	t.Helper()
	app.claimPaneFocus(FocusNavigation)
	app.app.SetFocus(target)
}

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

func TestClosingAnOverlayGoesBackToThePaneItOpenedFrom(t *testing.T) {
	overlays := []struct {
		name  string
		open  func(*App)
		close func(*App)
	}{
		{"palette", (*App).openPalette, (*App).closePalette},
		{"picker", func(a *App) {
			a.pickerModal.Show("Set Priority", []PickerItem{{ID: "1", Label: "Urgent"}}, func(PickerItem) {})
		}, func(a *App) { a.pickerModal.Hide() }},
		{"multi_select", func(a *App) {
			a.multiSelectModal.Show("Filter Labels", []MultiSelectItem{{ID: "1", Label: "Bug"}}, nil, func([]string) {})
		}, func(a *App) { a.multiSelectModal.Hide() }},
		{"keys", (*App).ShowKeysModal, func(a *App) { a.keysModal.Hide() }},
	}
	for _, overlay := range overlays {
		t.Run(overlay.name, func(t *testing.T) {
			app := newUXTestApp(t)
			app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
			app.app.SetRoot(app.pages, true)
			app.focusedPane = FocusIssues
			app.updateFocus()

			overlay.open(app)
			overlay.close(app)

			if app.focusedPane != FocusIssues {
				t.Fatalf("closing the %s left the pane on %v, want the issues pane it opened from", overlay.name, app.focusedPane)
			}
		})
	}
}

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

		app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
		if !app.navSearchFocused {
			t.Errorf("after %v, Tab did not return to the query box", key)
		}
	}
}

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

func TestATreeRebuildKeepsTheCursorOutWhileResultsShow(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me"})
	teams := []linearapi.Team{{ID: "team-1", Name: "Engineering"}}
	app.rebuildNavigationTree(teams, nil)
	app.focusNavSearch()
	app.performIssueSearch("found")
	waitForResults()

	app.rebuildNavigationTree(teams, nil)

	current := app.navigationTree.GetRoot().GetChildren()[0]
	if got := current.GetSelectedTextStyle(); got == selectionStyle(app.theme) {
		t.Error("rebuilding the tree relit the cursor while search results held the pane")
	}
}

func TestResetCachedStateLeavesNoArmedSearch(t *testing.T) {
	app := newUXTestApp(t)
	app.config.SearchDebounce = 10 * time.Millisecond
	fired := make(chan struct{}, 4)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case fired <- struct{}{}:
		default:
		}
	}
	app.navSearchInput.SetText("auth")

	app.resetCachedState()
	for len(fired) > 0 {
		<-fired
	}

	select {
	case <-fired:
		t.Fatal("a search fired after the reset, so emptying the box armed a debounce nothing cancels")
	case <-time.After(200 * time.Millisecond):
	}
}

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

func TestPickingANavigationNodeDropsTheSearch(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"})
	app.focusNavSearch()
	app.navSearchInput.SetText("found")
	app.performIssueSearch("found")
	waitForResults()

	release, done := make(chan struct{}), make(chan struct{})
	var finished sync.Once
	app.refreshCompleted = func() { finished.Do(func() { close(done) }) }
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		<-release
		return linearapi.IssuePage{}, nil
	}

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
	if got := app.issuesColumn.GetItem(0); got == tview.Primitive(app.searchResultsTable) {
		t.Error("the results table is still mounted while the picked list loads")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the refresh never finished")
	}
}

func TestThemeChangeRebuildsTheQueryBoxWithoutResearching(t *testing.T) {
	app := newUXTestApp(t)
	app.navSearchInput.SetText("found")

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
