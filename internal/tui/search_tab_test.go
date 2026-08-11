package tui

import (
	"context"
	"errors"
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

// TestSearchPaneStylesLightOneHalfAtATime covers the cue the tab had none of:
// the query box and the result list share a border, so a lit row and a live
// field read the same whichever holds the keyboard.
func TestSearchPaneStylesLightOneHalfAtATime(t *testing.T) {
	for _, theme := range []Theme{RosePineMoonTheme, HighContrastTheme, LinearTheme} {
		input := searchPaneStyles(theme, true, true)
		results := searchPaneStyles(theme, true, false)
		away := searchPaneStyles(theme, false, false)

		if input.LabelColor != theme.Accent || input.FieldText != theme.Foreground {
			t.Errorf("typing in the query box left it muted: label %v text %v", input.LabelColor, input.FieldText)
		}
		if _, _, attr := input.SelectedStyle.Decompose(); attr&tcell.AttrBold != 0 {
			t.Error("the list kept its live selection while the query box had the keyboard")
		}

		if results.LabelColor != theme.SecondaryText || results.FieldText != theme.SecondaryText {
			t.Errorf("the query box stayed lit while the list had the keyboard: label %v", results.LabelColor)
		}
		fg, bg, attr := results.SelectedStyle.Decompose()
		if fg != theme.SelectionText || bg != theme.SelectionBg || attr&tcell.AttrBold == 0 {
			t.Errorf("the live list selection = %v on %v attr %v, want the theme's selection", fg, bg, attr)
		}

		// Off the tab entirely, neither half claims the keyboard.
		if away.LabelColor != theme.SecondaryText {
			t.Error("the query box stayed lit with the pane unfocused")
		}
		if _, _, attr := away.SelectedStyle.Decompose(); attr&tcell.AttrBold != 0 {
			t.Error("the list stayed lit with the pane unfocused")
		}
	}
}

// TestTheMutedSelectionStaysFindable pins why the marker is an underline. Two
// shipped themes cannot carry it as a background: HighContrast sets HeaderBg to
// its Background, and rose_pine_moon's Background is the terminal's.
func TestTheMutedSelectionStaysFindable(t *testing.T) {
	for _, theme := range []Theme{RosePineMoonTheme, HighContrastTheme, LinearTheme} {
		muted := searchPaneStyles(theme, true, true).SelectedStyle
		if _, _, attr := muted.Decompose(); attr&tcell.AttrUnderline == 0 {
			t.Error("the muted selection has no marker, so Tab comes back to a row nobody can see")
		}
	}
}

// TestTabLeavesTheSearchInputForTheResults covers the query box's only exit
// that keeps the query. h and l type into it, and Esc empties it before it lets
// go, so without Tab there is no way out with the words still there.
func TestTabLeavesTheSearchInputForTheResults(t *testing.T) {
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
			Issues: []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "Found me", State: "Todo"}},
		}, nil
	}
	// Tab selects the first result, and the detail fetch that follows repaints
	// the pane titles from its own goroutine. Park it, or it races updateFocus
	// on the way out of the input.
	holdDetailFetches(t, app)

	app.openSearchTab()
	app.performIssueSearch("found")
	waitForDraw(t, drawn)

	if got := app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatal("Tab leaked past the search input")
	}
	if app.searchInputFocused {
		t.Error("Tab left the keyboard in the query box")
	}
	if got := len(app.searchIssueRows); got != 1 {
		t.Errorf("search rows = %d, want the results kept on the way out", got)
	}
	if app.activeIssuesSection != IssuesSectionSearch {
		t.Errorf("Tab left the Search tab for %v, want the results inside it", app.activeIssuesSection)
	}
}
