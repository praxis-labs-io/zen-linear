package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/config"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// stringPtr returns a string pointer for test helpers.
func stringPtr(value string) *string {
	return &value
}

// waitForCondition polls until a condition is true or times out.
func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

// TestRefreshIssues_LazyLoadsPages verifies first page renders before background pages.
func TestRefreshIssues_LazyLoadsPages(t *testing.T) {
	cfg := config.Config{
		PageSize: 2,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg)
	app.queueUpdateDraw = func(f func()) { f() }

	issue1 := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	issue2 := linearapi.Issue{ID: "issue-2", Identifier: "ABC-2", Title: "Second", State: "Todo"}

	issueByID := map[string]linearapi.Issue{
		issue1.ID: issue1,
		issue2.ID: issue2,
	}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return issueByID[id], nil
	}

	blockNext := make(chan struct{})
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		if after == nil {
			return linearapi.IssuePage{
				Issues:    []linearapi.Issue{issue1},
				HasNext:   true,
				EndCursor: stringPtr("cursor-1"),
			}, nil
		}
		<-blockNext
		return linearapi.IssuePage{
			Issues:  []linearapi.Issue{issue2},
			HasNext: false,
		}, nil
	}

	app.refreshIssues()

	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1
	})
	app.issuesMu.RLock()
	selectedIssue := app.selectedIssue
	app.issuesMu.RUnlock()
	if selectedIssue == nil || selectedIssue.ID != issue1.ID {
		t.Fatalf("selectedIssue = %#v, want %s", selectedIssue, issue1.ID)
	}

	close(blockNext)
	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 2
	})
	app.issuesMu.RLock()
	selectedIssue = app.selectedIssue
	app.issuesMu.RUnlock()
	if selectedIssue == nil || selectedIssue.ID != issue1.ID {
		t.Fatalf("selectedIssue after append = %#v, want %s", selectedIssue, issue1.ID)
	}
}

// TestRefreshIssues_CancelsStaleLoad verifies stale background pages are ignored.
func TestRefreshIssues_CancelsStaleLoad(t *testing.T) {
	cfg := config.Config{
		PageSize: 2,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg)
	app.queueUpdateDraw = func(f func()) { f() }

	issue1 := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	issue2 := linearapi.Issue{ID: "issue-2", Identifier: "ABC-2", Title: "Second", State: "Todo"}
	issue3 := linearapi.Issue{ID: "issue-3", Identifier: "ABC-3", Title: "Third", State: "Todo"}

	issueByID := map[string]linearapi.Issue{
		issue1.ID: issue1,
		issue2.ID: issue2,
		issue3.ID: issue3,
	}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return issueByID[id], nil
	}

	var mode atomic.Int32
	blockNext := make(chan struct{})
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		if mode.Load() == 0 {
			if after == nil {
				return linearapi.IssuePage{
					Issues:    []linearapi.Issue{issue1},
					HasNext:   true,
					EndCursor: stringPtr("cursor-1"),
				}, nil
			}
			<-blockNext
			return linearapi.IssuePage{
				Issues:  []linearapi.Issue{issue2},
				HasNext: false,
			}, nil
		}

		if after == nil {
			return linearapi.IssuePage{
				Issues:  []linearapi.Issue{issue3},
				HasNext: false,
			}, nil
		}

		return linearapi.IssuePage{}, nil
	}

	app.refreshIssues()
	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1
	})

	mode.Store(1)
	app.refreshIssues()
	close(blockNext)

	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1 && app.issues[0].ID == issue3.ID
	})
	app.issuesMu.RLock()
	issueID := app.issues[0].ID
	app.issuesMu.RUnlock()
	if issueID == issue2.ID {
		t.Fatalf("stale issue applied, got %s", issueID)
	}
}
