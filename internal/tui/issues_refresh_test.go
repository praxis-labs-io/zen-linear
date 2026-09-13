package tui

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func TestASupersededRefreshDoesNotPaintItsFirstPage(t *testing.T) {
	app := newUXTestApp(t)
	done := make(chan struct{})
	var finished sync.Once
	app.refreshCompleted = func() { finished.Do(func() { close(done) }) }
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{
			{ID: "late", Identifier: "ZEN-1", Title: "Landed after the bump"},
		}}, nil
	}

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

func TestASupersededRefreshDoesNotReportItsFailure(t *testing.T) {
	app := newUXTestApp(t)
	done := make(chan struct{})
	var finished sync.Once
	app.refreshCompleted = func() { finished.Do(func() { close(done) }) }
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{}, errors.New("the fetch that lost the race")
	}

	var bump sync.Once
	app.queueUpdateDraw = func(f func()) {
		bump.Do(func() { app.refreshGeneration.Add(1) })
		f()
	}

	app.refreshIssuesWithFocusChange(false)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the refresh never finished")
	}

	if app.issuesErr != nil {
		t.Errorf("a superseded refresh recorded %v as the list's error", app.issuesErr)
	}
	if app.isLoading {
		t.Error("a superseded refresh left the pane loading")
	}
}
