package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// tabLabels strips the color tags the strip is built from, leaving the labels
// in the order they render.
func tabLabels(t *testing.T, app *App) []string {
	t.Helper()
	title := app.issuesTabsTitle(false)
	var labels []string
	for _, segment := range strings.Split(title, tabSeparator) {
		clean := strings.TrimSpace(stripColorTags(segment))
		if clean != "" {
			labels = append(labels, clean)
		}
	}
	return labels
}

// pressKey sends a rune through the app's real input capture, so a handler that
// claims the key before the focused pane sees it shows up here.
func pressKey(app *App, r rune) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

// holdDetailFetches parks every background detail fetch until the test ends, so
// its queueUpdateDraw stub cannot repaint the UI alongside the assertions.
func holdDetailFetches(t *testing.T, app *App) {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		<-release
		return linearapi.Issue{ID: id}, nil
	}
}

func stripColorTags(s string) string {
	for {
		open := strings.Index(s, "[")
		if open < 0 {
			return s
		}
		end := strings.Index(s[open:], "]")
		if end < 0 {
			return s
		}
		s = s[:open] + s[open+end+1:]
	}
}

func TestIssuesTabsTitle_AllLeadsAndMyStaysAtZero(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", AssigneeID: "user-1", Assignee: "Me"},
	})

	// The three tabs are fixed. An empty My reads (0) rather than vanishing and
	// shifting the tabs beside it.
	got := tabLabels(t, app)
	want := []string{"All (2)", "My (0)", "Search"}
	if !equalLabels(got, want) {
		t.Fatalf("tab strip without a current user = %v, want %v", got, want)
	}

	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssuesTables("")

	got = tabLabels(t, app)
	want = []string{"All (2)", "My (1)", "Search"}
	if !equalLabels(got, want) {
		t.Fatalf("tab strip with My populated = %v, want %v", got, want)
	}
}

func TestCycleIssuesSection_OrdersAllMyThenSearch(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", AssigneeID: "user-1", Assignee: "Me"},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssuesTables("")
	// Landing on a tab selects a row, and that detail fetch would repaint the
	// pane titles from its own goroutine while the next cycle repaints them here.
	holdDetailFetches(t, app)

	if app.activeIssuesSection != IssuesSectionAll {
		t.Fatalf("a fresh list opens on %v, want All", app.activeIssuesSection)
	}

	forward := []IssuesSection{IssuesSectionMy, IssuesSectionSearch, IssuesSectionAll}
	for _, want := range forward {
		app.cycleIssuesSection(1)
		if app.activeIssuesSection != want {
			t.Fatalf("cycling forward reached %v, want %v", app.activeIssuesSection, want)
		}
	}

	backward := []IssuesSection{IssuesSectionSearch, IssuesSectionMy, IssuesSectionAll}
	for _, want := range backward {
		app.cycleIssuesSection(-1)
		if app.activeIssuesSection != want {
			t.Fatalf("cycling backward reached %v, want %v", app.activeIssuesSection, want)
		}
	}
}

// ZNL-16: the row index came from the All model and the selection was applied
// to the Other table, so the parent jump landed on a row nobody was looking at.
func TestJumpToParent_SelectsTheParentInTheTabOnScreen(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent"}
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "parent-1", Identifier: "LIN-1", Title: "Parent", AssigneeID: "user-2",
			Children: []linearapi.IssueChildRef{{ID: "child-1", Identifier: "LIN-2", Title: "Child"}},
		},
		{ID: "child-1", Identifier: "LIN-2", Title: "Child", AssigneeID: "user-1", Assignee: "Me", Parent: parentRef},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssuesTables("child-1")
	holdDetailFetches(t, app)
	app.toggleIssueExpanded("parent-1")

	table := app.tableForSection(IssuesSectionAll)
	childRow := app.getRowForIssueInSection("child-1", IssuesSectionAll)
	if childRow < 1 {
		t.Fatal("child has no row in All after expanding its parent")
	}
	table.Select(childRow, 0)
	app.issuesMu.Lock()
	app.selectedIssue = app.allIDToIssue["child-1"]
	app.issuesMu.Unlock()
	app.focusedPane = FocusIssues

	pressKey(app, 'p')

	if app.activeIssuesSection != IssuesSectionAll {
		t.Fatalf("view_parent switched to %v, want to stay on All", app.activeIssuesSection)
	}
	wantRow := app.getRowForIssueInSection("parent-1", IssuesSectionAll)
	if row, _ := table.GetSelection(); row != wantRow {
		t.Fatalf("All table selection = row %d, want the parent at row %d", row, wantRow)
	}
	if got := app.GetSelectedIssue(); got == nil || got.ID != "parent-1" {
		t.Fatalf("selected issue = %v, want parent-1", got)
	}
}

// A sub-issue whose parent is out of the fetched scope is the common case for a
// filtered list. Without feedback the key looks broken rather than out of reach.
func TestViewParent_SaysSoWhenTheParentIsNotLoaded(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "child-1", Identifier: "LIN-2", Title: "Child",
			Parent: &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent"},
		},
	})
	holdDetailFetches(t, app)
	app.focusedPane = FocusIssues

	pressKey(app, 'p')

	if got := app.statusBar.GetText(true); !strings.Contains(got, "Parent issue not loaded") {
		t.Fatalf("status after view_parent with an unfetched parent = %q, want the parent-not-loaded feedback", got)
	}
}

// A child assigned to the user whose parent is not is an orphan in My. p there
// has to fall back to All rather than sit on a parent My cannot show.
func TestParentKeyFallsBackToAllWhenMyHasNoParent(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent"}
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "parent-1", Identifier: "LIN-1", Title: "Parent", AssigneeID: "user-2"},
		{ID: "child-1", Identifier: "LIN-2", Title: "Child", AssigneeID: "user-1", Assignee: "Me", Parent: parentRef},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssuesTables("child-1")
	holdDetailFetches(t, app)

	app.activeIssuesSection = IssuesSectionMy
	app.updateIssuesColumnLayout()
	app.focusedPane = FocusIssues
	myTable := app.tableForSection(IssuesSectionMy)
	myTable.Select(app.getRowForIssueInSection("child-1", IssuesSectionMy), 0)
	app.issuesMu.Lock()
	app.selectedIssue = app.myIDToIssue["child-1"]
	app.issuesMu.Unlock()

	pressKey(app, 'p')

	if app.activeIssuesSection != IssuesSectionAll {
		t.Fatalf("p from My landed on %v, want All, which holds the parent", app.activeIssuesSection)
	}
	allTable := app.tableForSection(IssuesSectionAll)
	wantRow := app.getRowForIssueInSection("parent-1", IssuesSectionAll)
	if row, _ := allTable.GetSelection(); row != wantRow {
		t.Fatalf("All table selection = row %d, want the parent at row %d", row, wantRow)
	}
}

// h and l are pane movement in the issues list, as the README documents. The
// table used to carry its own expand/collapse branches for them, which the
// global handler swallowed before the table ever ran.
func TestIssuesListLeavesHAndLToPaneMovement(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	holdDetailFetches(t, app)
	app.detailsHidden = false
	app.focusedPane = FocusIssues

	pressKey(app, 'h')
	if app.focusedPane != FocusNavigation {
		t.Fatalf("h from the issues list landed on %v, want Navigation", app.focusedPane)
	}

	app.focusedPane = FocusIssues
	pressKey(app, 'l')
	if app.focusedPane != FocusDetails {
		t.Fatalf("l from the issues list landed on %v, want Details", app.focusedPane)
	}
}

func equalLabels(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Empty tabs became reachable when My stopped hiding itself. Landing on one has
// to drop the selection, or status, assign and archive act on the issue the
// previous tab had selected, invisibly.
func TestJumpToSection_EmptyTabDropsTheSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssuesTables("issue-1")
	holdDetailFetches(t, app)

	app.issuesMu.Lock()
	app.selectedIssue = app.allIDToIssue["issue-1"]
	app.issuesMu.Unlock()
	if len(app.myIssueRows) != 0 {
		t.Fatalf("fixture has %d My rows, want an empty My tab", len(app.myIssueRows))
	}

	app.cycleIssuesSection(1)

	if app.activeIssuesSection != IssuesSectionMy {
		t.Fatalf("cycling landed on %v, want the empty My tab", app.activeIssuesSection)
	}
	if got := app.GetSelectedIssue(); got != nil {
		t.Fatalf("selected issue on an empty tab = %v, want nil", got)
	}
}

// rebuildIssuesTables resolves through allIDToIssue, whose values point into a
// snapshot of the list that the next rebuild replaces.
func TestRebuildIssuesTables_ReturnsACopyNotAnAlias(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	holdDetailFetches(t, app)

	selected := app.rebuildIssuesTables("issue-1")
	if selected == nil {
		t.Fatal("rebuildIssuesTables returned nil for a loaded issue")
	}
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	for i := range app.issues {
		if selected == &app.issues[i] {
			t.Fatal("returned issue aliases the issues backing array")
		}
	}
}

// A tab owed a deferred render carries the row it should land on. Restoring a
// remembered index over it drops the cursor on whatever now sits there.
func TestCycleIssuesSection_KeepsTheDeferredRenderSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha", AssigneeID: "user-1", Assignee: "Me"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", AssigneeID: "user-1", Assignee: "Me"},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssueRowModels()
	holdDetailFetches(t, app)

	// Park My's cursor on row 1, then defer a render that wants row 2.
	app.activeIssuesSection = IssuesSectionAll
	app.tableForSection(IssuesSectionMy).Select(1, 0)
	app.renderIssueSections(map[IssuesSection]string{IssuesSectionMy: "issue-2"})
	if _, pending := app.pendingSectionRenders[IssuesSectionMy]; !pending {
		t.Fatal("My was painted rather than deferred; the test needs a pending render")
	}

	app.cycleIssuesSection(1)

	if app.activeIssuesSection != IssuesSectionMy {
		t.Fatalf("cycling landed on %v, want My", app.activeIssuesSection)
	}
	want := app.getRowForIssueInSection("issue-2", IssuesSectionMy)
	if row, _ := app.tableForSection(IssuesSectionMy).GetSelection(); row != want {
		t.Fatalf("My selection = row %d, want row %d from the deferred render", row, want)
	}
}
