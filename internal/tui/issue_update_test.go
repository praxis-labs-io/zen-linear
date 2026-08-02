package tui

import (
	"context"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// titleColumn is the index of the title cell in DefaultIssueColumns.
const titleColumn = 3

func newIssueUpdateTestApp(t *testing.T, issues []linearapi.Issue) *App {
	t.Helper()
	app := newUXTestApp()
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		t.Error("a single-issue update refetched the whole list")
		return linearapi.IssuePage{}, nil
	}
	app.issuesMu.Lock()
	app.issues = issues
	app.selectedIssue = &app.issues[0]
	app.issuesMu.Unlock()
	app.rebuildIssuesTables(issues[0].ID)
	return app
}

func TestApplyIssueUpdate_RepaintsOneRowWhenNothingMoves(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})

	// Park the cursor and the horizontal scroll somewhere a full re-render
	// would reset.
	table := app.tableForSection(IssuesSectionOther)
	table.Select(2, 0)
	table.SetOffset(0, 2)

	updated := linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha renamed"}
	app.applyIssueUpdate(updated)

	if got := table.GetCell(1, titleColumn).Text; got != "Alpha renamed" {
		t.Fatalf("row 1 title = %q, want %q", got, "Alpha renamed")
	}
	if row, _ := table.GetSelection(); row != 2 {
		t.Fatalf("selected row = %d, want 2", row)
	}
	if _, columnOffset := table.GetOffset(); columnOffset != 2 {
		t.Fatalf("column offset = %d, want 2", columnOffset)
	}
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if app.issues[0].Title != "Alpha renamed" {
		t.Fatalf("local issue title = %q, want %q", app.issues[0].Title, "Alpha renamed")
	}
}

func TestApplyIssueUpdate_KeepsCommentsTheMutationDoesNotReturn(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Comments:    []linearapi.Comment{{ID: "comment-1", Body: "keep me"}},
			Subscribers: []linearapi.User{{ID: "user-9"}},
		},
	})

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha renamed"})

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if len(app.issues[0].Comments) != 1 {
		t.Fatalf("comments = %#v, want the loaded comment kept", app.issues[0].Comments)
	}
	if len(app.issues[0].Subscribers) != 1 {
		t.Fatalf("subscribers = %#v, want the loaded subscriber kept", app.issues[0].Subscribers)
	}
}

func TestApplyIssueUpdate_MovesIssueBetweenSectionsWithoutRefetch(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}

	app.applyIssueUpdate(linearapi.Issue{
		ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
		AssigneeID: "user-1", Assignee: "Me",
	})

	if _, ok := app.myIDToIssue["issue-1"]; !ok {
		t.Fatalf("issue-1 missing from My Issues: %#v", app.myIDToIssue)
	}
	if app.activeIssuesSection != IssuesSectionMy {
		t.Fatalf("active section = %v, want My", app.activeIssuesSection)
	}
	if got := app.tableForSection(IssuesSectionMy).GetCell(1, titleColumn).Text; got != "Alpha" {
		t.Fatalf("My Issues row 1 title = %q, want Alpha", got)
	}
}

func TestApplyIssueUpdate_RefetchesWhenIssueIsNotInTheList(t *testing.T) {
	app := newUXTestApp()
	refreshDone := installRefreshCompletionHook(app)
	fetched := make(chan struct{}, 1)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		select {
		case fetched <- struct{}{}:
		default:
		}
		return linearapi.IssuePage{}, nil
	}

	app.applyIssueUpdate(linearapi.Issue{ID: "unknown", Identifier: "LIN-9"})

	select {
	case <-fetched:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the fallback refetch")
	}
	// The refresh runs on its own goroutine and touches tview globals; letting
	// it outlive the test races the next test's app construction.
	waitForRefreshCompletion(t, refreshDone)
}

func TestApplyIssueRemoval_DropsRowAndLandsOnTheNextIssue(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma"},
	})

	app.applyIssueRemoval("issue-2")

	app.issuesMu.RLock()
	remaining := make([]string, 0, len(app.issues))
	for _, issue := range app.issues {
		remaining = append(remaining, issue.ID)
	}
	selected := app.selectedIssue
	app.issuesMu.RUnlock()

	if len(remaining) != 2 || remaining[0] != "issue-1" || remaining[1] != "issue-3" {
		t.Fatalf("remaining issues = %v, want [issue-1 issue-3]", remaining)
	}
	if selected == nil || selected.ID != "issue-3" {
		t.Fatalf("selected issue = %#v, want issue-3", selected)
	}
	table := app.tableForSection(IssuesSectionOther)
	if got := table.GetCell(2, titleColumn).Text; got != "Gamma" {
		t.Fatalf("row 2 title = %q, want Gamma", got)
	}
}

func TestApplyIssueRemoval_DetachesTheRowFromItsParent(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Children: []linearapi.IssueChildRef{{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}},
		},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", Parent: parentRef},
	})

	app.applyIssueRemoval("issue-2")

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if len(app.issues[0].Children) != 0 {
		t.Fatalf("parent children = %#v, want the archived child dropped", app.issues[0].Children)
	}
}

func TestApplyIssueUpdate_MovesTheChildRefBetweenParents(t *testing.T) {
	oldParent := &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Children: []linearapi.IssueChildRef{{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma"}},
		},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", Parent: oldParent},
	})

	newParent := &linearapi.IssueRef{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}
	app.applyIssueUpdate(linearapi.Issue{
		ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", Parent: newParent,
	})

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	byID := make(map[string]linearapi.Issue, len(app.issues))
	for _, issue := range app.issues {
		byID[issue.ID] = issue
	}
	if len(byID["issue-1"].Children) != 0 {
		t.Fatalf("old parent children = %#v, want empty", byID["issue-1"].Children)
	}
	if children := byID["issue-2"].Children; len(children) != 1 || children[0].ID != "issue-3" {
		t.Fatalf("new parent children = %#v, want issue-3", children)
	}
}

func TestApplyIssueUpdate_DropsTheChildRefWhenTheParentIsRemoved(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Children: []linearapi.IssueChildRef{{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}},
		},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", Parent: parentRef},
	})

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"})

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if len(app.issues[0].Children) != 0 {
		t.Fatalf("parent children = %#v, want empty after the child left", app.issues[0].Children)
	}
}

func TestApplyIssueInsert_AddsTheCreatedRowAndSelectsIt(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})

	app.applyIssueInsert(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"})

	app.issuesMu.RLock()
	count := len(app.issues)
	selected := app.selectedIssue
	app.issuesMu.RUnlock()

	if count != 2 {
		t.Fatalf("issue count = %d, want 2", count)
	}
	if selected == nil || selected.ID != "issue-2" {
		t.Fatalf("selected issue = %#v, want issue-2", selected)
	}
	if _, ok := app.idToIssue["issue-2"]; !ok {
		t.Fatal("created issue missing from the row model")
	}
}

func TestApplyIssueInsert_LinksASubIssueToItsParent(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})

	app.applyIssueInsert(linearapi.Issue{
		ID: "issue-2", Identifier: "LIN-2", Title: "Beta",
		Parent: &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	for _, issue := range app.issues {
		if issue.ID != "issue-1" {
			continue
		}
		if len(issue.Children) != 1 || issue.Children[0].ID != "issue-2" {
			t.Fatalf("parent children = %#v, want issue-2", issue.Children)
		}
		return
	}
	t.Fatal("parent issue missing from the list")
}

func TestRenderIssueSections_DefersOffScreenTabsUntilShown(t *testing.T) {
	app := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha", AssigneeID: "user-1", Assignee: "Me"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}
	app.rebuildIssueRowModels()

	app.activeIssuesSection = IssuesSectionOther
	app.renderIssueSections(map[IssuesSection]string{
		IssuesSectionMy:    "issue-1",
		IssuesSectionOther: "issue-2",
	})

	if _, pending := app.pendingSectionRenders[IssuesSectionMy]; !pending {
		t.Fatal("My Issues rendered while off screen")
	}
	if got := app.tableForSection(IssuesSectionOther).GetCell(1, titleColumn).Text; got != "Beta" {
		t.Fatalf("Other Issues row 1 title = %q, want Beta", got)
	}

	app.activeIssuesSection = IssuesSectionMy
	app.updateIssuesColumnLayout()

	if _, pending := app.pendingSectionRenders[IssuesSectionMy]; pending {
		t.Fatal("My Issues still pending after being shown")
	}
	if got := app.tableForSection(IssuesSectionMy).GetCell(1, titleColumn).Text; got != "Alpha" {
		t.Fatalf("My Issues row 1 title = %q, want Alpha", got)
	}
}
