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

	app.navigationTree = tview.NewTreeView()
	app.issuesColumn = tview.NewFlex()
	app.detailsView = tview.NewFlex()
	app.contentFlex = tview.NewFlex()
	for _, pane := range []tview.Primitive{app.navigationTree, app.issuesColumn, app.detailsView} {
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

	_, nav = paneRect(app.navigationTree)
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

// TestMediumPaneSplitKeepsNavShare covers the two-pane layout, where the nav
// pane must hold the same sixth of the width it had before the wide ratio
// changed.
func TestMediumPaneSplitKeepsNavShare(t *testing.T) {
	app := &App{focusedPane: FocusNavigation}
	nav, issues, details := paneWidths(t, app, 90)

	if app.layoutMode != layoutMedium {
		t.Fatalf("layoutMode = %v, want layoutMedium", app.layoutMode)
	}
	if details != 0 {
		t.Errorf("details width = %d, want the pane left out", details)
	}
	if nav != 15 || issues != 75 {
		t.Errorf("split = %d | %d; want 15 | 75", nav, issues)
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
