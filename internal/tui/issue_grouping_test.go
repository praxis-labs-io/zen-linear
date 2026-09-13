package tui

import (
	"context"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func hydratedGroupingApp(t *testing.T) *App {
	t.Helper()
	app := newUXTestApp(t)
	holdDetailFetches(t, app)
	app.config.GroupBy = GroupByStatus

	hydrated := linearapi.Issue{
		ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo",
		Comments:    []linearapi.Comment{{ID: "comment-1", Body: "still here"}},
		Attachments: []linearapi.Attachment{{ID: "attachment-1", Title: "PR"}},
	}
	app.issuesMu.Lock()
	app.issues = []linearapi.Issue{
		{ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo"},
		{ID: "issue-2", Identifier: "ZNL-2", Title: "Second", State: "Done"},
	}
	app.selectedIssue = &hydrated
	app.issuesMu.Unlock()
	app.rebuildIssuesTables("issue-1")
	return app
}

func assertSelectionStaysHydrated(t *testing.T, app *App) {
	t.Helper()
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if app.selectedIssue == nil || app.selectedIssue.ID != "issue-1" {
		t.Fatalf("selectedIssue = %#v, want issue-1", app.selectedIssue)
	}
	if got := len(app.selectedIssue.Comments); got != 1 {
		t.Fatalf("selected issue has %d comments, want 1: the rebuild replaced it with the list copy", got)
	}
	if got := len(app.selectedIssue.Attachments); got != 1 {
		t.Fatalf("selected issue has %d attachments, want 1: the rebuild replaced it with the list copy", got)
	}
}

func TestRegroupIssues_KeepsTheHydratedSelection(t *testing.T) {
	app := hydratedGroupingApp(t)

	app.config.GroupBy = GroupByPriority
	app.regroupIssues("Grouped by priority")

	assertSelectionStaysHydrated(t, app)
}

func TestToggleGroupCollapse_KeepsTheHydratedSelection(t *testing.T) {
	app := hydratedGroupingApp(t)

	var header IssueRow
	for _, row := range app.rowsForSection(IssuesSectionList) {
		if row.IsHeader {
			header = row
			break
		}
	}
	if header.HeaderKey == "" {
		t.Fatal("no group header in the All rows")
	}
	app.toggleGroupCollapse(IssuesSectionList, header)

	assertSelectionStaysHydrated(t, app)
}

func TestRegroupIssues_FetchesDetailsForANewSelection(t *testing.T) {
	app := newUXTestApp(t)
	fetched := make(chan string, 1)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		select {
		case fetched <- id:
		default:
		}
		return linearapi.Issue{ID: id}, nil
	}
	app.config.GroupBy = GroupByStatus
	app.issuesMu.Lock()
	app.issues = []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo"}}
	app.issuesMu.Unlock()

	app.regroupIssues("Grouped by status")

	select {
	case id := <-fetched:
		if id != "issue-1" {
			t.Fatalf("fetched %q, want issue-1", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the new selection never fetched its details")
	}
}
