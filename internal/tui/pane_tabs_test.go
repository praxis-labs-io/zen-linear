package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// pressKey sends a rune through the app's real input capture, so a handler that
// claims the key before the focused pane sees it shows up here.
func pressKey(app *App, r rune) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

// runPaletteCommand runs a command by id off the same registry the palette
// lists, which is the only way to reach one that carries no default key.
func runPaletteCommand(t *testing.T, app *App, id string) {
	t.Helper()
	for _, cmd := range app.paletteCtrl.commands {
		if cmd.ID == id {
			cmd.Run(app)
			return
		}
	}
	t.Fatalf("command %q not registered", id)
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

// ZNL-16: the row index came from one section model and the selection was
// applied to another table, so the parent jump landed on a row nobody was
// looking at.
func TestJumpToParent_SelectsTheParentInTheSectionOnScreen(t *testing.T) {
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

	table := app.tableForSection(IssuesSectionList)
	childRow := app.getRowForIssueInSection("child-1", IssuesSectionList)
	if childRow < 1 {
		t.Fatal("child has no row in the list after expanding its parent")
	}
	table.Select(childRow, 0)
	app.issuesMu.Lock()
	app.selectedIssue = app.listIDToIssue["child-1"]
	app.issuesMu.Unlock()
	app.focusedPane = FocusIssues

	runPaletteCommand(t, app, "view_parent")

	if app.activeIssuesSection != IssuesSectionList {
		t.Fatalf("view_parent switched to %v, want to stay on the list", app.activeIssuesSection)
	}
	wantRow := app.getRowForIssueInSection("parent-1", IssuesSectionList)
	if row, _ := table.GetSelection(); row != wantRow {
		t.Fatalf("list selection = row %d, want the parent at row %d", row, wantRow)
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

	runPaletteCommand(t, app, "view_parent")

	if got := statusText(app); !strings.Contains(got, "Parent issue not loaded") {
		t.Fatalf("status after view_parent with an unfetched parent = %q, want the parent-not-loaded feedback", got)
	}
}

// Search results are a flat list, so a parent there is not a tree move. The
// jump has to fall back to the navigation list, which holds every fetched
// issue, rather than sit on a row search cannot show.
func TestViewParentFallsBackToTheListFromSearchResults(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent"}
	child := linearapi.Issue{ID: "child-1", Identifier: "LIN-2", Title: "Child", Parent: parentRef}
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "parent-1", Identifier: "LIN-1", Title: "Parent"},
		child,
	})
	app.rebuildIssuesTables("child-1")
	holdDetailFetches(t, app)

	app.searchQuery = "child"
	app.navSearchInput.SetText("child")
	app.searchIssues = []linearapi.Issue{child}
	app.searchIssueRows, app.searchIDToIssue = buildFlatSearchRows(app.searchIssues)
	app.activeIssuesSection = IssuesSectionSearch
	app.renderIssueSections(map[IssuesSection]string{IssuesSectionSearch: "child-1"})
	app.updateIssuesColumnLayout()
	app.focusedPane = FocusIssues
	app.issuesMu.Lock()
	app.selectedIssue = app.searchIDToIssue["child-1"]
	app.issuesMu.Unlock()

	runPaletteCommand(t, app, "view_parent")

	if app.activeIssuesSection != IssuesSectionList {
		t.Fatalf("view_parent from search landed on %v, want the list, which holds the parent", app.activeIssuesSection)
	}
	wantRow := app.getRowForIssueInSection("parent-1", IssuesSectionList)
	if row, _ := app.listIssuesTable.GetSelection(); row != wantRow {
		t.Fatalf("list selection = row %d, want the parent at row %d", row, wantRow)
	}
	// Leaving the results for the list is leaving the search. A query left in
	// the box describes a pane showing something else, and the session then
	// saves a query alongside a list issue.
	if got := app.navSearchInput.GetText(); got != "" {
		t.Errorf("the query survived the jump to the list: %q", got)
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

// Landing on an empty section has to drop the selection, or status, assign and
// archive act on the issue the previous one had selected, invisibly.
func TestJumpToSection_AnEmptySectionDropsTheSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	app.rebuildIssuesTables("issue-1")
	holdDetailFetches(t, app)

	app.issuesMu.Lock()
	app.selectedIssue = app.listIDToIssue["issue-1"]
	app.issuesMu.Unlock()
	if len(app.searchIssueRows) != 0 {
		t.Fatalf("fixture has %d search rows, want none", len(app.searchIssueRows))
	}

	app.jumpToSection(IssuesSectionSearch, 0)

	if app.activeIssuesSection != IssuesSectionSearch {
		t.Fatalf("the jump landed on %v, want the empty search section", app.activeIssuesSection)
	}
	if got := app.GetSelectedIssue(); got != nil {
		t.Fatalf("selected issue on an empty section = %v, want nil", got)
	}
}

// rebuildIssuesTables resolves through listIDToIssue, whose values point into a
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

// A section owed a deferred render carries the row it should land on. Restoring
// a remembered index over it drops the cursor on whatever now sits there.
func TestJumpToSection_KeepsTheDeferredRenderSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	app.rebuildIssueRowModels()
	holdDetailFetches(t, app)

	// Park the list's cursor on row 1, then defer a render that wants row 2.
	app.listIssuesTable.Select(1, 0)
	app.activeIssuesSection = IssuesSectionSearch
	app.renderIssueSections(map[IssuesSection]string{IssuesSectionList: "issue-2"})
	if _, pending := app.pendingSectionRenders[IssuesSectionList]; !pending {
		t.Fatal("the list was painted rather than deferred; the test needs a pending render")
	}

	app.jumpToSection(IssuesSectionList, 0)

	if app.activeIssuesSection != IssuesSectionList {
		t.Fatalf("the jump landed on %v, want the list", app.activeIssuesSection)
	}
	want := app.getRowForIssueInSection("issue-2", IssuesSectionList)
	if row, _ := app.listIssuesTable.GetSelection(); row != want {
		t.Fatalf("list selection = row %d, want row %d from the deferred render", row, want)
	}
}
