package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// newLoadingPaneTestApp builds an App whose issue fetch is held open, so a test
// can read what the panes say while a fetch is out. The panes paint their
// message when the load starts, so nothing here waits on a frame.
func newLoadingPaneTestApp(t *testing.T, page linearapi.IssuePage, fetchErr error) (*App, chan struct{}) {
	t.Helper()

	app := newDefaultNavTestApp(t, config.Config{})
	t.Cleanup(func() { app.setIssuesLoading(false) })

	release := make(chan struct{})
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		<-release
		return page, fetchErr
	}
	return app, release
}

// mountedIssuesPane is the primitive the issues column is showing.
func mountedIssuesPane(t *testing.T, app *App) tview.Primitive {
	t.Helper()
	if app.issuesColumn.GetItemCount() == 0 {
		t.Fatal("issues column is empty")
	}
	return app.issuesColumn.GetItem(0)
}

// placeholderText is the message in the pane, without its color tags.
func placeholderText(app *App) string {
	return app.issuesPlaceholderText.GetText(true)
}

// TestIssuesPaneNamesWhatItIsWaitingOn covers the launch complaint: an empty
// table reads as a broken app, so the pane says it is loading and then shows
// the rows.
func TestIssuesPaneNamesWhatItIsWaitingOn(t *testing.T) {
	page := linearapi.IssuePage{Issues: []linearapi.Issue{
		{ID: "issue-1", Identifier: "ENG-1", Title: "First"},
	}}
	app, release := newLoadingPaneTestApp(t, page, nil)
	refreshDone := installRefreshCompletionHook(app)

	app.refreshIssuesWithFocusChange(false)

	if got := mountedIssuesPane(t, app); got != tview.Primitive(app.issuesPlaceholder) {
		t.Fatalf("mounted %T while loading, want the placeholder", got)
	}
	if text := placeholderText(app); !strings.Contains(text, "Loading issues") {
		t.Fatalf("placeholder = %q, want it to name the fetch", text)
	}
	if !strings.Contains(placeholderText(app), spinnerFramesDots[0]) {
		t.Fatalf("placeholder = %q, want a spinner glyph", placeholderText(app))
	}

	close(release)
	waitForRefreshCompletion(t, refreshDone)

	if got := mountedIssuesPane(t, app); got != tview.Primitive(app.listIssuesTable) {
		t.Fatalf("mounted %T once rows arrived, want the table", got)
	}
}

// TestIssuesPaneFocusFollowsTheSwap covers the pane going dead when the
// placeholder and the table trade places under it: focus rides on the
// primitive, so the one that leaves the layout takes the keys with it.
func TestIssuesPaneFocusFollowsTheSwap(t *testing.T) {
	withIssues := linearapi.IssuePage{Issues: []linearapi.Issue{
		{ID: "issue-1", Identifier: "ENG-1", Title: "First"},
	}}

	tests := []struct {
		name  string
		first linearapi.IssuePage
		then  linearapi.IssuePage
		want  func(app *App) tview.Primitive
	}{
		{
			name: "rows arrive",
			then: withIssues,
			want: func(app *App) tview.Primitive { return app.listIssuesTable },
		},
		{
			name:  "rows go away",
			first: withIssues,
			want:  func(app *App) tview.Primitive { return app.issuesPlaceholder },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page := tc.first
			app, release := newLoadingPaneTestApp(t, linearapi.IssuePage{}, nil)
			app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
				return page, nil
			}
			// SetRoot primes the delegate tview moves focus with, so a harness
			// without it cannot see focus leave a pane.
			app.app.SetRoot(app.pages, true)
			close(release)
			refreshDone := installRefreshCompletionHook(app)

			app.refreshIssuesWithFocusChange(false)
			waitForRefreshCompletion(t, refreshDone)
			app.focusedPane = FocusIssues
			app.updateFocus()

			page = tc.then
			// A workspace switch, a session restore, and a navigation change all
			// refresh without a focus change of their own.
			app.refreshIssuesWithFocusChange(false)
			waitForRefreshCompletion(t, refreshDone)

			if got := app.app.GetFocus(); got != tc.want(app) {
				t.Fatalf("keyboard focus landed on %T, want what the pane is mounting", got)
			}
			if got := mountedIssuesPane(t, app); got != app.app.GetFocus() {
				t.Fatalf("mounted %T while focus is on %T", got, app.app.GetFocus())
			}
		})
	}
}

// TestIssuesPaneStartsAsLoading covers the window between the layout being
// built and the first fetch starting, which flashed "No issues" at every
// launch.
func TestIssuesPaneStartsAsLoading(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{})

	if text := placeholderText(app); !strings.Contains(text, "Loading issues") {
		t.Fatalf("placeholder = %q, want it to read as loading before the first fetch", text)
	}
}

// TestIssuesPaneShowsTheFetchFailure covers the pane a failed launch leaves
// behind, which otherwise says "No issues" and blames the workspace.
func TestIssuesPaneShowsTheFetchFailure(t *testing.T) {
	app, release := newLoadingPaneTestApp(t, linearapi.IssuePage{}, errors.New("no route to host"))
	refreshDone := installRefreshCompletionHook(app)

	app.refreshIssuesWithFocusChange(false)
	close(release)
	waitForRefreshCompletion(t, refreshDone)

	if got := mountedIssuesPane(t, app); got != tview.Primitive(app.issuesPlaceholder) {
		t.Fatalf("mounted %T after a failure, want the placeholder", got)
	}
	text := placeholderText(app)
	if !strings.Contains(text, "Could not load issues") || !strings.Contains(text, "no route to host") {
		t.Fatalf("placeholder = %q, want the failure", text)
	}
}

// TestIssuesPaneSaysEmptyWhenNothingIsInFlight covers the honestly empty list.
func TestIssuesPaneSaysEmptyWhenNothingIsInFlight(t *testing.T) {
	app, release := newLoadingPaneTestApp(t, linearapi.IssuePage{}, nil)
	refreshDone := installRefreshCompletionHook(app)

	app.refreshIssuesWithFocusChange(false)
	close(release)
	waitForRefreshCompletion(t, refreshDone)

	if text := placeholderText(app); !strings.Contains(text, "No issues") {
		t.Fatalf("placeholder = %q, want the empty state", text)
	}
}

// TestDetailsPaneSaysLoadingWhileTheListLoads covers the second empty pane at
// launch: nothing is selected yet because nothing has arrived.
func TestDetailsPaneSaysLoadingWhileTheListLoads(t *testing.T) {
	page := linearapi.IssuePage{Issues: []linearapi.Issue{
		{ID: "issue-1", Identifier: "ENG-1", Title: "First"},
	}}
	app, release := newLoadingPaneTestApp(t, page, nil)
	refreshDone := installRefreshCompletionHook(app)

	app.refreshIssuesWithFocusChange(false)

	if text := app.detailsDescriptionView.GetText(true); !strings.Contains(text, "Loading issue") {
		t.Fatalf("details pane = %q, want it to name the fetch", text)
	}

	close(release)
	waitForRefreshCompletion(t, refreshDone)

	if text := app.detailsDescriptionView.GetText(true); !strings.Contains(text, "ENG-1") {
		t.Fatalf("details pane = %q, want the selected issue", text)
	}
}

// TestLoadingIndicatorStopsWithTheLastFetch verifies the frame loop is not left
// spinning over panes that already have their answer.
func TestLoadingIndicatorStopsWithTheLastFetch(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{})

	app.setNavLoading(true)
	if !app.loading.running() {
		t.Fatal("frame loop is stopped while the navigation fetch is out")
	}

	app.setIssuesLoading(true)
	app.setNavLoading(false)
	if !app.loading.running() {
		t.Fatal("frame loop stopped while the issue fetch is still out")
	}

	app.setIssuesLoading(false)
	if app.loading.running() {
		t.Fatal("frame loop still running with nothing in flight")
	}
}

// TestSupersededRefreshLeavesTheLoadingFlagAlone covers the refresh that lands
// after a newer one took over: clearing the flag there stops the spinner and
// drops the pane to "No issues" while a page is still on its way.
func TestSupersededRefreshLeavesTheLoadingFlagAlone(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{})

	app.loadingGeneration = 7
	app.setIssuesLoading(true)

	app.finishIssuesLoad(6, errors.New("stale failure"))

	if !app.isLoading {
		t.Fatal("a superseded refresh cleared the newer refresh's loading flag")
	}
	if app.issuesErr != nil {
		t.Fatalf("issuesErr = %v, want the stale failure discarded", app.issuesErr)
	}

	app.finishIssuesLoad(7, nil)
	if app.isLoading {
		t.Fatal("the owning refresh did not settle the pane")
	}
}

// TestFocusLandsOnThePlaceholderWhenMounted covers Tab into an empty issues
// pane: focusing the detached table leaves no pane looking focused.
func TestFocusLandsOnThePlaceholderWhenMounted(t *testing.T) {
	app, release := newLoadingPaneTestApp(t, linearapi.IssuePage{Issues: []linearapi.Issue{
		{ID: "issue-1", Identifier: "ENG-1", Title: "First"},
	}}, nil)
	refreshDone := installRefreshCompletionHook(app)

	app.focusedPane = FocusIssues
	app.updateFocus()

	if got := app.app.GetFocus(); got != tview.Primitive(app.issuesPlaceholder) {
		t.Fatalf("focus landed on %T while the pane was empty, want the placeholder", got)
	}

	app.refreshIssuesWithFocusChange(false)
	close(release)
	waitForRefreshCompletion(t, refreshDone)
	app.focusedPane = FocusIssues
	app.updateFocus()

	if got := app.app.GetFocus(); got != tview.Primitive(app.listIssuesTable) {
		t.Fatalf("focus landed on %T once rows arrived, want the table", got)
	}
}

// TestNavigationFailureAnswersTheWaitingNode covers an offline cold launch: a
// spinner frozen over "Loading teams" reads as still working.
func TestNavigationFailureAnswersTheWaitingNode(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{})

	app.reportNavigationFailure(errors.New("no route to host"))

	if app.navLoadingNode == nil {
		t.Fatal("the tree has no waiting node")
	}
	if text := app.navLoadingNode.GetText(); strings.Contains(text, "Loading") {
		t.Fatalf("waiting node = %q, want it to report the failure", text)
	}
}
