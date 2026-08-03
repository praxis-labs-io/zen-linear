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
	for _, segment := range strings.Split(title, "·") {
		clean := strings.TrimSpace(stripColorTags(segment))
		if clean != "" {
			labels = append(labels, clean)
		}
	}
	return labels
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

func TestIssuesTabsTitle_AllLeadsAndMyHidesWhenEmpty(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", AssigneeID: "user-1", Assignee: "Me"},
	})

	got := tabLabels(t, app)
	want := []string{"All (2)", "Search"}
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

	viewParent := findCommandByID(DefaultCommands(app), "view_parent")
	if viewParent == nil {
		t.Fatal("view_parent command not found")
	}
	viewParent.Run(app)

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

// A child assigned to the user whose parent is not is an orphan in My. Pressing
// h there has to fall back to All rather than sit on a parent My cannot show.
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
	myTable := app.tableForSection(IssuesSectionMy)
	myTable.Select(app.getRowForIssueInSection("child-1", IssuesSectionMy), 0)

	app.handleIssuesTableRune(myTable, IssuesSectionMy, tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModNone))

	if app.activeIssuesSection != IssuesSectionAll {
		t.Fatalf("h from My landed on %v, want All, which holds the parent", app.activeIssuesSection)
	}
	allTable := app.tableForSection(IssuesSectionAll)
	wantRow := app.getRowForIssueInSection("parent-1", IssuesSectionAll)
	if row, _ := allTable.GetSelection(); row != wantRow {
		t.Fatalf("All table selection = row %d, want the parent at row %d", row, wantRow)
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
