package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// zoomKey presses the zoom shortcut through the real global handler, so the
// per-pane dispatch that has to reach the command is part of what is covered.
func zoomKey(app *App) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'v', tcell.ModNone))
}

func newZoomTestApp(t *testing.T) *App {
	t.Helper()

	app := newUXTestApp(t)
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "ZNL-35", Title: "Focused details view"}
	return app
}

// The zoom is reachable from every pane, because it is a palette command and
// all three pane handlers fall through to the shortcut lookup.
func TestZoomFiresFromEveryPane(t *testing.T) {
	for _, pane := range []FocusTarget{FocusNavigation, FocusIssues, FocusDetails} {
		app := newZoomTestApp(t)
		app.detailsHidden = false
		app.focusedPane = pane

		zoomKey(app)
		if !app.detailsZoomed {
			t.Errorf("v from pane %v did not zoom", pane)
		}
		zoomKey(app)
		if app.detailsZoomed {
			t.Errorf("v from pane %v did not unzoom", pane)
		}
	}
}

func TestZoomFocusesTheDetailsPane(t *testing.T) {
	app := newZoomTestApp(t)
	app.focusedPane = FocusIssues

	zoomKey(app)
	if app.focusedPane != FocusDetails {
		t.Errorf("focusedPane = %v after zooming, want FocusDetails", app.focusedPane)
	}
	if app.detailsHidden {
		t.Error("zoom left the details pane hidden")
	}
}

// Zooming an empty pane would show a full-width "No issue selected".
func TestZoomNeedsASelectedIssue(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues

	zoomKey(app)
	if app.detailsZoomed {
		t.Error("zoomed with no issue selected")
	}
}

// Tab must not offer a pane the zoom has taken off screen.
func TestTabSkipsTheIssuesPaneWhileZoomed(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true
	app.layoutMode = layoutWide

	for _, pane := range app.visiblePanes() {
		if pane == FocusIssues {
			t.Fatal("visiblePanes offered the issues pane while zoomed")
		}
	}

	tabKey(app, false)
	if app.focusedPane == FocusIssues {
		t.Error("Tab landed on the issues pane while zoomed")
	}
}

// Below the wide breakpoint the nav tree is not mounted either, so it must not
// be offered.
func TestTabHoldsTheDetailsPaneWhileZoomedAndNarrow(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true
	app.layoutMode = layoutNarrow

	tabKey(app, false)
	if app.focusedPane != FocusDetails {
		t.Errorf("focusedPane = %v, want the zoom to hold FocusDetails", app.focusedPane)
	}
}

// Asking for the issues pane by number is the other way out of the zoom.
func TestPaneNumberTwoReleasesTheZoom(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '2', tcell.ModNone))

	if app.detailsZoomed {
		t.Error("2 did not release the zoom")
	}
	if app.focusedPane != FocusIssues {
		t.Errorf("focusedPane = %v, want FocusIssues", app.focusedPane)
	}
}

// Escape leaves the zoom without also closing the details pane, which is what
// Enter does when the pane is a rail.
func TestEscapeReleasesTheZoomAndKeepsThePane(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if app.detailsZoomed {
		t.Error("Escape did not release the zoom")
	}
	if app.detailsHidden {
		t.Error("Escape closed the details pane as well as the zoom")
	}
	if app.focusedPane != FocusIssues {
		t.Errorf("focusedPane = %v, want FocusIssues", app.focusedPane)
	}
}

// Left and h mean "back to the list", which the zoom is covering.
func TestLeftReleasesTheZoom(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))

	if app.detailsZoomed {
		t.Error("Left did not release the zoom")
	}
	if app.focusedPane != FocusIssues {
		t.Errorf("focusedPane = %v, want FocusIssues", app.focusedPane)
	}
}

func TestZoomHonoursAKeybindingOverride(t *testing.T) {
	app := NewApp(linearapi.ClientConfig{}, config.Config{
		PageSize:    1,
		Keybindings: map[string]string{"zoom_details": "V"},
	}, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	app.selectedIssue = &linearapi.Issue{ID: "issue-1"}
	app.focusedPane = FocusIssues

	zoomKey(app)
	if app.detailsZoomed {
		t.Error("v still zoomed after the binding moved to V")
	}
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'V', tcell.ModNone))
	if !app.detailsZoomed {
		t.Error("V did not zoom after being bound to it")
	}
}
