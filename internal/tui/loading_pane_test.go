package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// newLoadingPaneTestApp builds an App whose issue fetch is held open, so a test
// can read what the panes say while a fetch is out. The frame loop is parked at
// an hour: the panes paint their message when the load starts, and a tick
// landing mid-assertion would only be a race.
func newLoadingPaneTestApp(t *testing.T, page linearapi.IssuePage, fetchErr error) (*App, chan struct{}) {
	t.Helper()

	app := newDefaultNavTestApp(config.Config{})
	stopDetailTimersOnCleanup(t, app)
	app.loadingFrameDelay = time.Hour
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

	if got := mountedIssuesPane(t, app); got != tview.Primitive(app.allIssuesTable) {
		t.Fatalf("mounted %T once rows arrived, want the table", got)
	}
}

// TestIssuesPaneStartsAsLoading covers the window between the layout being
// built and the first fetch starting, which flashed "No issues" at every
// launch.
func TestIssuesPaneStartsAsLoading(t *testing.T) {
	app := newDefaultNavTestApp(config.Config{})
	stopDetailTimersOnCleanup(t, app)

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
	app := newDefaultNavTestApp(config.Config{})
	app.loadingFrameDelay = time.Hour

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
