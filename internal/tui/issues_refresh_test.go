package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// TestASupersededRefreshDoesNotPaintItsFirstPage guards the suite's flakes: the
// first page's closure rebuilds the tables, unguarded by the generation check.
func TestASupersededRefreshDoesNotPaintItsFirstPage(t *testing.T) {
	app := newUXTestApp(t)
	done := make(chan struct{})
	app.refreshCompleted = func() { close(done) }
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{
			{ID: "late", Identifier: "ZEN-1", Title: "Landed after the bump"},
		}}, nil
	}

	// The bump lands while the first page's closure is queued, which is the
	// window the check on the fetching side cannot cover.
	var bump sync.Once
	app.queueUpdateDraw = func(f func()) {
		bump.Do(func() { app.refreshGeneration.Add(1) })
		f()
	}

	baseline := app.listIssuesTable.GetRowCount()
	app.refreshIssuesWithFocusChange(false)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the refresh never finished")
	}

	app.issuesMu.RLock()
	count := len(app.issues)
	app.issuesMu.RUnlock()
	if count != 0 {
		t.Fatalf("a superseded refresh painted %d issues", count)
	}
	if got := app.listIssuesTable.GetRowCount(); got != baseline {
		t.Fatalf("a superseded refresh took the table from %d rows to %d", baseline, got)
	}
}
