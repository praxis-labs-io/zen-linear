package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// paneRect returns a pane's left edge and width as of the last draw.
func paneRect(pane tview.Primitive) (left, width int) {
	left, _, width, _ = pane.GetRect()
	return left, width
}

// paneWidths builds a content flex of bare panes and draws it at the given
// terminal width, returning the width each pane was laid out at. Every pane
// starts zeroed, so one the layout left out reads as 0 rather than as the
// tview default rect.
func paneWidths(t *testing.T, app *App, width int) (nav, issues, details int) {
	t.Helper()

	app.navigationPanel = tview.NewFlex()
	app.issuesColumn = tview.NewFlex()
	app.detailsView = tview.NewFlex()
	app.contentFlex = tview.NewFlex()
	for _, pane := range []tview.Primitive{app.navigationPanel, app.issuesColumn, app.detailsView} {
		pane.SetRect(0, 0, 0, 0)
	}

	app.layoutMode = layoutModeForWidth(width)
	app.rebuildContentLayout()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, 40)

	app.contentFlex.SetRect(0, 0, width, 40)
	app.contentFlex.Draw(screen)

	_, nav = paneRect(app.navigationPanel)
	_, issues = paneRect(app.issuesColumn)
	_, details = paneRect(app.detailsView)
	return nav, issues, details
}

// TestWidePaneSplit pins the three-pane ratio at 6:15:10.
func TestWidePaneSplit(t *testing.T) {
	app := &App{}
	nav, issues, details := paneWidths(t, app, 180)

	if app.layoutMode != layoutWide {
		t.Fatalf("layoutMode = %v, want layoutWide", app.layoutMode)
	}
	if nav != 34 || issues != 87 || details != 59 {
		t.Errorf("split = %d | %d | %d; want 34 | 87 | 59", nav, issues, details)
	}
	if left, _ := paneRect(app.issuesColumn); left != nav {
		t.Errorf("issues pane starts at %d, want %d right after the nav pane", left, nav)
	}
}

// TestWidePaneSplitWithDetailsHidden covers toggling details off, where the
// nav pane keeps the share it had before it got a bump for the three-pane
// case.
func TestWidePaneSplitWithDetailsHidden(t *testing.T) {
	app := &App{detailsHidden: true}
	nav, issues, details := paneWidths(t, app, 180)

	if details != 0 {
		t.Errorf("details width = %d, want the pane left out", details)
	}
	if nav != 45 || issues != 135 {
		t.Errorf("split = %d | %d; want 45 | 135", nav, issues)
	}
}

// TestMediumPaneSplitLeavesTheDetailsPaneOut covers the two-pane layout, where
// the details pane appears only when it is the focused one.
func TestMediumPaneSplitLeavesTheDetailsPaneOut(t *testing.T) {
	app := &App{focusedPane: FocusNavigation}
	nav, issues, details := paneWidths(t, app, 90)

	if app.layoutMode != layoutMedium {
		t.Fatalf("layoutMode = %v, want layoutMedium", app.layoutMode)
	}
	if details != 0 {
		t.Errorf("details width = %d, want the pane left out", details)
	}
	if nav != navWidthMedium || issues != 90-navWidthMedium {
		t.Errorf("split = %d | %d; want %d | %d", nav, issues, navWidthMedium, 90-navWidthMedium)
	}
}

// TestTheNavPaneKeepsItsWidthAcrossTheTwoPaneRange covers what a share did to
// it. Proportional, the pane ran from 18 columns at the top of the range to 11
// at the bottom, and a tree that narrow is a column of ellipses.
func TestTheNavPaneKeepsItsWidthAcrossTheTwoPaneRange(t *testing.T) {
	for _, width := range []int{70, 90, 109} {
		app := &App{focusedPane: FocusNavigation}
		nav, issues, _ := paneWidths(t, app, width)
		if app.layoutMode != layoutMedium {
			t.Fatalf("width %d: layoutMode = %v, want layoutMedium", width, app.layoutMode)
		}
		if nav != navWidthMedium {
			t.Errorf("width %d: nav = %d, want %d", width, nav, navWidthMedium)
		}
		if issues != width-navWidthMedium {
			t.Errorf("width %d: issues = %d, want %d", width, issues, width-navWidthMedium)
		}
	}
}

// TestNarrowLayoutGivesTheFocusedPaneEverything covers the one-pane layout.
func TestNarrowLayoutGivesTheFocusedPaneEverything(t *testing.T) {
	app := &App{focusedPane: FocusNavigation}
	nav, issues, _ := paneWidths(t, app, 60)

	if app.layoutMode != layoutNarrow {
		t.Fatalf("layoutMode = %v, want layoutNarrow", app.layoutMode)
	}
	if nav != 60 {
		t.Errorf("nav width = %d, want the full 60", nav)
	}
	if issues != 0 {
		t.Errorf("issues width = %d, want the pane left out", issues)
	}
}

// TestZoomedLayoutDropsTheIssuesColumn covers the zoomed reading view: the
// issues list goes, the nav tree stays as a spine.
func TestZoomedLayoutDropsTheIssuesColumn(t *testing.T) {
	app := &App{detailsZoomed: true, focusedPane: FocusDetails}
	nav, issues, details := paneWidths(t, app, 180)

	if issues != 0 {
		t.Errorf("issues width = %d, want the pane left out", issues)
	}
	if nav != 41 || details != 139 {
		t.Errorf("split = %d | %d; want 41 | 139", nav, details)
	}
}

// TestZoomedLayoutDropsTheNavBelowWide covers the narrower terminals, where a
// nav tree beside the reading measure does not fit.
func TestZoomedLayoutDropsTheNavBelowWide(t *testing.T) {
	for _, width := range []int{90, 60} {
		app := &App{detailsZoomed: true, focusedPane: FocusDetails}
		nav, issues, details := paneWidths(t, app, width)

		if nav != 0 || issues != 0 {
			t.Errorf("at %d: nav = %d, issues = %d; want both left out", width, nav, issues)
		}
		if details != width {
			t.Errorf("at %d: details width = %d, want the full width", width, details)
		}
	}
}

// TestHidingTheDetailsPaneIsInertWhileZoomed guards the arrangement that would
// otherwise mount nothing at all, and it guards it by refusing rather than by
// unzooming. The key used to end the zoom as a side effect of hiding, which
// read as a hide key doing something it never claims to do.
func TestHidingTheDetailsPaneIsInertWhileZoomed(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.detailsZoomed = true
	app.focusedPane = FocusDetails

	app.toggleDetailsPane()

	if !app.detailsZoomed {
		t.Error("hiding the details pane released the zoom, want the key inert")
	}
	if app.detailsHidden {
		t.Error("the details pane hid itself while zoomed, leaving nothing mounted")
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("focus moved to %v, want it to stay in the zoomed pane", app.focusedPane)
	}
}
