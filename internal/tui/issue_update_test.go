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
