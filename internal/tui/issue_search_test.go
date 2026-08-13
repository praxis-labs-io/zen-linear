package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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

// A canceled request answers with context.Canceled, and every keystroke cancels
// one. Delivered as a result, that reads "Search failed" between every two
// letters typed.
func TestPerformIssueSearch_ASupersededFetchIsNotAFailure(t *testing.T) {
	app := newUXTestApp(t)
	// The cancel this covers is synchronous; the search behind it is not, and
	// left armed it paints from its own goroutine after the test returns.
	app.config.SearchDebounce = time.Hour
	drawn := make(chan struct{}, 8)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	started := make(chan struct{}, 1)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		if params.Search == "a" {
			started <- struct{}{}
			<-ctx.Done()
			return linearapi.IssuePage{}, ctx.Err()
		}
		return linearapi.IssuePage{Issues: []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me"}}}, nil
	}

	app.performIssueSearch("a")
	<-started
	// Typing the next letter cancels the first request, the way the debounce
	// does on every keystroke.
	app.scheduleSearchDebounce("ab")
	waitForDraw(t, drawn)

	if app.searchErr != nil {
		t.Fatalf("searchErr = %v after a superseded fetch, want none", app.searchErr)
	}
	if message, _ := app.issuesPlaceholderMessage(); strings.Contains(message, "failed") {
		t.Fatalf("the pane says %q while a newer query is in flight", message)
	}
}

// The spinner is a ticker, not a glyph: nothing in tview has a frame loop, so a
// waiting state that does not run the loop paints one frozen frame and reads as
// a hang.
func TestSearchRunsTheSpinner(t *testing.T) {
	app := newUXTestApp(t)
	app.config.SearchDebounce = time.Hour
	started := make(chan struct{}, 1)
	// Canceling is what frees the parked fetch, and it bumps the generation
	// too, so what the fetch delivers on its way out is discarded rather than
	// painted into the next test's app.
	t.Cleanup(func() { app.cancelSearchFetch() })
	app.fetchIssuesPage = func(ctx context.Context, _ linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		started <- struct{}{}
		<-ctx.Done()
		return linearapi.IssuePage{}, ctx.Err()
	}

	app.performIssueSearch("auth")
	<-started

	if !app.loading.running() {
		t.Fatal("the frame loop is stopped while a search is out, so the spinner cannot advance")
	}
	first, _ := app.issuesPlaceholderMessage()
	// The indicator is seeded with frame zero and the first advance returns it
	// again, so the glyph only moves on the second.
	app.loading.advance()
	app.loading.advance()
	if second, _ := app.issuesPlaceholderMessage(); second == first {
		t.Errorf("the waiting message did not change with the frame: %q", second)
	}

	app.setSearchLoading(false)
	if app.loading.running() {
		t.Error("the frame loop kept running with nothing in flight, so it queues draws forever")
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

// TestPerformIssueSearch_OwnsWhatTheIssuesPaneShows pins the one place the
// section moves. Anywhere else deciding it is how the pane and the query box
// come to disagree.
func TestPerformIssueSearch_OwnsWhatTheIssuesPaneShows(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"})

	app.performIssueSearch("found")
	waitForResults()
	if app.activeIssuesSection != IssuesSectionSearch {
		t.Fatalf("a query left the pane on %v, want the results", app.activeIssuesSection)
	}

	app.performIssueSearch("")
	if app.activeIssuesSection != IssuesSectionList {
		t.Fatalf("an empty query left the pane on %v, want the list back", app.activeIssuesSection)
	}
}

// TestSearchStatesReachThePlaceholder covers the messages that used to live in
// the Search tab's own panel. They belong to the shared placeholder now, so a
// failed or empty search still says what happened.
func TestSearchStatesReachThePlaceholder(t *testing.T) {
	app := newUXTestApp(t)
	app.activeIssuesSection = IssuesSectionSearch

	app.searchLoading = true
	if message, _ := app.issuesPlaceholderMessage(); !strings.Contains(message, "Searching") {
		t.Errorf("a search in flight says %q", message)
	}

	app.searchLoading = false
	if message, _ := app.issuesPlaceholderMessage(); !strings.Contains(message, "No results") {
		t.Errorf("a search that found nothing says %q", message)
	}

	app.searchErr = errors.New("boom")
	if message, _ := app.issuesPlaceholderMessage(); !strings.Contains(message, "Search failed") {
		t.Errorf("a failed search says %q", message)
	}
}

// TestEnterLeavesTheQueryBoxForTheResults covers the box's only exit that keeps
// the query. Letters type, Down goes to the tree, and Esc empties the query
// before it lets go, so without Enter there is no way to the results with the
// words still there.
func TestEnterLeavesTheQueryBoxForTheResults(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"})
	// Enter selects the first result, and the detail fetch that follows
	// repaints the pane titles from its own goroutine. Park it, or it races
	// updateFocus on the way out of the box.
	holdDetailFetches(t, app)

	app.focusNavSearch()
	app.performIssueSearch("found")
	waitForResults()

	if got := app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); got != nil {
		t.Fatal("Enter leaked past the query box")
	}
	if app.navSearchFocused {
		t.Error("Enter left the keyboard in the query box")
	}
	if app.focusedPane != FocusIssues {
		t.Errorf("Enter left focus on %v, want the results", app.focusedPane)
	}
	if got := len(app.searchIssueRows); got != 1 {
		t.Errorf("search rows = %d, want the results kept on the way out", got)
	}

	// Esc is the way back, matching the Enter that left.
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !app.navSearchFocused || app.focusedPane != FocusNavigation {
		t.Error("Esc did not return the keyboard to the query box")
	}
}

// TestEnterWithNoResultsKeepsTheKeyboard covers the pane going dead: with
// nothing mounted to receive it, moving focus anyway leaves the keys on a
// primitive that is not on screen.
func TestEnterWithNoResultsKeepsTheKeyboard(t *testing.T) {
	app := newUXTestApp(t)
	app.focusNavSearch()

	if got := app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)); got != nil {
		t.Fatal("Enter leaked past the query box")
	}
	if !app.navSearchFocused || app.focusedPane != FocusNavigation {
		t.Error("Enter with no results moved the keyboard off the query box")
	}
}

// newSearchTestApp returns an app whose search fetch answers with the given
// issues, and a wait for the queued draw that lands them. The fetch runs on its
// own goroutine, so the results are not readable when performIssueSearch
// returns.
func newSearchTestApp(t *testing.T, issues ...linearapi.Issue) (*App, func()) {
	t.Helper()
	app := newUXTestApp(t)
	drawn := make(chan struct{}, 8)
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: issues}, nil
	}
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	return app, func() {
		t.Helper()
		waitForDraw(t, drawn)
	}
}
