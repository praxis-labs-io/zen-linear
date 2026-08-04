package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// TestPerformIssueSearch_CancelsTheSupersededFetch guards the reason the search
// path threads a context at all: the generation counter already throws away a
// stale result, but without cancellation the request keeps running against the
// API, so a fast typist leaves one live query per debounce window.
func TestPerformIssueSearch_CancelsTheSupersededFetch(t *testing.T) {
	app := newUXTestApp(t)

	started := make(chan struct{}, 2)
	observed := make(chan error, 2)
	app.fetchIssuesPage = func(ctx context.Context, _ linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		started <- struct{}{}
		<-ctx.Done()
		observed <- ctx.Err()
		return linearapi.IssuePage{}, ctx.Err()
	}

	app.performIssueSearch("first")
	waitForDraw(t, started)

	// A newer query supersedes the first.
	app.performIssueSearch("second")
	waitForDraw(t, started)

	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first fetch saw %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the superseded fetch was never canceled")
	}

	// Clearing the query has to cancel the survivor too.
	app.performIssueSearch("")
	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("second fetch saw %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("clearing the query left a fetch running")
	}
}

func TestPerformIssueSearch_RendersResults(t *testing.T) {
	app := newUXTestApp(t)
	// Search state is UI-thread-only, so the test reads it after the queued
	// draw rather than polling it from here.
	drawn := make(chan struct{}, 8)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{
			Issues: []linearapi.Issue{
				{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"},
			},
		}, nil
	}

	app.performIssueSearch("found")
	waitForDraw(t, drawn)

	if got := len(app.searchIssueRows); got != 1 {
		t.Fatalf("search rows = %d, want 1", got)
	}
	if got := app.searchResultsTable.GetCell(1, titleColumn).Text; got != "Found me" {
		t.Fatalf("search result title = %q, want %q", got, "Found me")
	}
}
