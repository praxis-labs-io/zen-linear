package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

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

func TestPerformIssueSearch_ASupersededFetchIsNotAFailure(t *testing.T) {
	app := newUXTestApp(t)
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
	app.scheduleSearchDebounce("ab")
	waitForDraw(t, drawn)

	if app.searchErr != nil {
		t.Fatalf("searchErr = %v after a superseded fetch, want none", app.searchErr)
	}
	if message, _ := app.issuesPlaceholderMessage(); strings.Contains(message, "failed") {
		t.Fatalf("the pane says %q while a newer query is in flight", message)
	}
}

func TestSearchRunsTheSpinner(t *testing.T) {
	app := newUXTestApp(t)
	app.config.SearchDebounce = time.Hour
	started := make(chan struct{}, 1)
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

func TestAFailedSearchDropsTheRowsItReplaces(t *testing.T) {
	app := newUXTestApp(t)
	app.config.SearchDebounce = time.Hour
	drawn := make(chan struct{}, 8)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	fail := false
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		if fail {
			return linearapi.IssuePage{}, errNotReachable
		}
		return linearapi.IssuePage{Issues: []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me"}}}, nil
	}

	app.performIssueSearch("found")
	waitForDraw(t, drawn)
	if len(app.searchIssueRows) != 1 {
		t.Fatalf("search rows = %d, want the first query's result", len(app.searchIssueRows))
	}

	fail = true
	app.performIssueSearch("boom")
	waitForDraw(t, drawn)

	if got := len(app.searchIssueRows); got != 0 {
		t.Errorf("search rows = %d after a failure, want the stale results dropped", got)
	}
	if message, _ := app.issuesPlaceholderMessage(); !strings.Contains(message, "Search failed") {
		t.Errorf("the pane says %q, want the failure named", message)
	}
}

func TestResultsLandAsTheSelection(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-9", Identifier: "ZNL-9", Title: "Found me"})
	holdDetailFetches(t, app)
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "The list issue"}
	app.issuesMu.Unlock()

	app.performIssueSearch("found")
	waitForResults()

	if got := app.GetSelectedIssue(); got == nil || got.ID != "issue-9" {
		t.Errorf("selected issue = %v, want the result the pane lit", got)
	}
}

func TestClearingResultsDropsTheRestoredIssue(t *testing.T) {
	app := newUXTestApp(t)
	app.pendingSearchIssueID = "issue-1"

	app.clearSearchResults()

	if app.pendingSearchIssueID != "" {
		t.Errorf("pendingSearchIssueID = %q, want it dropped with the results", app.pendingSearchIssueID)
	}
}

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

func TestEnterLeavesTheQueryBoxForTheResults(t *testing.T) {
	app, waitForResults := newSearchTestApp(t, linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"})
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

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !app.navSearchFocused || app.focusedPane != FocusNavigation {
		t.Error("Esc did not return the keyboard to the query box")
	}
}

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
