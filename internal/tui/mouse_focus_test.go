package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func newMouseTestApp(t *testing.T) *App {
	t.Helper()
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.issues = []linearapi.Issue{
		{ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo"},
		{ID: "issue-2", Identifier: "ZNL-2", Title: "Second", State: "Todo"},
	}
	app.rebuildIssuesTables("issue-1")
	app.selectedIssue = &app.issues[0]
	app.updateDetailsView()
	app.detailsHidden = false
	return app
}

func layOut(t *testing.T, app *App, width, height int, start FocusTarget) tcell.SimulationScreen {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	app.app.SetScreen(screen)
	screen.SetSize(width, height)
	app.app.SetRoot(app.pages, true)
	app.focusPane(start)
	app.app.ForceDraw()
	if app.focusedPane != start {
		t.Fatalf("laying out %d wide left the pane on %v, want %v", width, app.focusedPane, start)
	}
	return screen
}

func clickAt(t *testing.T, app *App, x, y int) {
	t.Helper()
	press := tcell.NewEventMouse(x, y, tcell.ButtonPrimary, tcell.ModNone)
	deliver(t, app, press, tview.MouseLeftDown)

	release := tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone)
	if deliver(t, app, release, tview.MouseLeftUp) == nil {
		release = nil
	}
	deliver(t, app, release, tview.MouseLeftClick)
}

func deliver(t *testing.T, app *App, event *tcell.EventMouse, action tview.MouseAction) *tcell.EventMouse {
	t.Helper()
	event, action = app.handleMouse(event, action)
	if event == nil {
		return nil
	}
	app.pages.MouseHandler()(action, event, func(p tview.Primitive) { app.app.SetFocus(p) })
	return event
}

func paneCenter(t *testing.T, pane tview.Primitive) (int, int) {
	t.Helper()
	x, y, width, height := pane.GetRect()
	if width < 3 || height < 3 {
		t.Fatalf("pane was never drawn: rect %d,%d %dx%d", x, y, width, height)
	}
	return x + width/2, y + height/2
}

func rowCell(t *testing.T, table *tview.Table, row int) (int, int) {
	t.Helper()
	x, top, _, height := table.GetInnerRect()
	for y := top; y < top+height; y++ {
		if at, _ := table.CellAt(x, y); at == row {
			return x, y
		}
	}
	t.Fatalf("row %d is not on screen", row)
	return 0, 0
}

func borderColorAt(t *testing.T, screen tcell.SimulationScreen, x, y int) tcell.Color {
	t.Helper()
	cells, width, height := screen.GetContents()
	if x >= width || y >= height {
		t.Fatalf("cell %d,%d is off a %dx%d screen", x, y, width, height)
	}
	color, _, _ := cells[y*width+x].Style.Decompose()
	return color
}

func assertPaneBorders(t *testing.T, app *App, focused FocusTarget) {
	t.Helper()
	for _, pane := range []struct {
		name   string
		border tcell.Color
		want   FocusTarget
	}{
		{"navigation", app.navigationPanel.GetBorderColor(), FocusNavigation},
		{"issues", app.listIssuesTable.GetBorderColor(), FocusIssues},
		{"details", app.detailsView.GetBorderColor(), FocusDetails},
	} {
		want := app.theme.Border
		if pane.want == focused {
			want = app.theme.BorderFocus
		}
		if pane.border != want {
			t.Errorf("%s border = %v, want %v with %v focused", pane.name, pane.border, want, focused)
		}
	}
}

func TestClickingAPaneMovesTheKeys(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusNavigation)

	x, y := paneCenter(t, app.issuesColumn)
	clickAt(t, app, x, y)
	if app.focusedPane != FocusIssues {
		t.Fatalf("clicking the issues list left the pane on %v", app.focusedPane)
	}
	assertPaneBorders(t, app, FocusIssues)
	if focus := app.app.GetFocus(); focus != tview.Primitive(app.listIssuesTable) {
		t.Errorf("the keyboard is on %T, not the issues table", focus)
	}

	x, y = paneCenter(t, app.detailsView)
	clickAt(t, app, x, y)
	if app.focusedPane != FocusDetails {
		t.Fatalf("clicking the details pane left the pane on %v", app.focusedPane)
	}
	assertPaneBorders(t, app, FocusDetails)

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModNone))
	if app.focusedPane != FocusIssues {
		t.Errorf("h after the click landed on %v, so keys still route to the pane the mouse left", app.focusedPane)
	}
}

func TestClickingAPaneBorderRepaintsIt(t *testing.T) {
	app := newMouseTestApp(t)
	screen := layOut(t, app, 180, 40, FocusIssues)

	left, top, _, height := app.navigationPanel.GetRect()
	clickAt(t, app, left, top+height/2)

	if app.focusedPane != FocusNavigation {
		t.Fatalf("clicking the navigation border left the pane on %v", app.focusedPane)
	}
	painted := borderColorAt(t, screen, left, top+height/2)
	if painted != app.theme.BorderFocus {
		t.Errorf("the border on screen is %v, want %v: the click recolored nothing anyone can see", painted, app.theme.BorderFocus)
	}
}

func TestClickingAPaneThatReflowsTheLayoutOnlyReflowsIt(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 90, 40, FocusDetails)
	if app.layoutMode != layoutMedium {
		t.Fatalf("layout mode = %v, want medium", app.layoutMode)
	}

	before := app.navigationTree.GetCurrentNode()
	clickAt(t, app, 5, 5)

	if app.focusedPane != FocusIssues {
		t.Fatalf("clicking the issues list left the pane on %v", app.focusedPane)
	}
	if after := app.navigationTree.GetCurrentNode(); after != before {
		t.Error("the click reached the navigation tree that moved in under it")
	}
}

func TestAClickCannotTakeThePaneFromAnOverlay(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusIssues)

	app.openPalette()
	x, y := paneCenter(t, app.navigationPanel)
	clickAt(t, app, x, y)
	if app.focusedPane != FocusPalette {
		t.Errorf("a click behind the palette moved the pane to %v", app.focusedPane)
	}
	app.closePalette()

	app.ShowSettingsModal()
	if app.activeModal() == nil {
		t.Fatal("the settings modal never opened")
	}
	before := app.focusedPane
	clickAt(t, app, x, y)
	if app.focusedPane != before {
		t.Errorf("a click behind a modal moved the pane from %v to %v", before, app.focusedPane)
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	if page := app.pages.GetPage("settings"); page != nil && !page.HasFocus() {
		t.Errorf("the keyboard is on %T, outside the modal that owns it", app.app.GetFocus())
	}
}

func TestOnlyALeftPressMovesThePane(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusIssues)
	x, y := paneCenter(t, app.navigationPanel)

	for _, action := range []tview.MouseAction{
		tview.MouseMove,
		tview.MouseScrollUp,
		tview.MouseScrollDown,
		tview.MouseLeftUp,
		tview.MouseRightDown,
	} {
		event := tcell.NewEventMouse(x, y, tcell.ButtonPrimary, tcell.ModNone)
		forwarded, _ := app.handleMouse(event, action)
		if forwarded != event {
			t.Errorf("%v was swallowed", action)
		}
		if app.focusedPane != FocusIssues {
			t.Fatalf("%v moved the pane to %v", action, app.focusedPane)
		}
	}

	if forwarded, _ := app.handleMouse(nil, tview.MouseLeftDown); forwarded != nil {
		t.Error("a nil event came back non-nil")
	}
}

func TestClickingTheStatusRowKeepsTheKeysWithThePane(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusIssues)
	focus := app.app.GetFocus()

	clickAt(t, app, 20, 39)

	if app.focusedPane != FocusIssues {
		t.Errorf("clicking the status row moved the pane to %v", app.focusedPane)
	}
	if app.app.GetFocus() != focus {
		t.Errorf("clicking the status row moved the keyboard to %T", app.app.GetFocus())
	}
}

func TestClickingIntoAWritingBoxTakesTheBoxAndNotJustThePane(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusIssues)
	app.openComposeBox()
	app.app.ForceDraw()

	x, y := paneCenter(t, app.detailsComposeArea)
	app.focusPane(FocusIssues)
	app.app.ForceDraw()
	clickAt(t, app, x, y)

	if app.focusedPane != FocusDetails {
		t.Fatalf("clicking the compose box left the pane on %v", app.focusedPane)
	}
	if !app.detailsFocus.isWriting() {
		t.Errorf("comments focus = %v, want the writing box", app.detailsFocus)
	}
	if focus := app.app.GetFocus(); focus != tview.Primitive(app.detailsComposeArea) {
		t.Errorf("the keyboard is on %T, not the compose box", focus)
	}

	writing := app.detailsFocus
	left, top, _, height := app.detailsView.GetRect()
	clickAt(t, app, left, top+height/2)
	if app.detailsFocus != writing {
		t.Errorf("clicking the details border moved the box focus to %v", app.detailsFocus)
	}
}

func TestClickingAnIssueRowStillSelectsIt(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusNavigation)

	rows := app.rowsForSection(IssuesSectionList)
	target := nextIssueRow(rows, 1, 1)
	if target < 1 {
		t.Fatal("the issues fixture has no second row to click")
	}
	x, y := rowCell(t, app.listIssuesTable, target)

	clickAt(t, app, x, y)

	if app.focusedPane != FocusIssues {
		t.Fatalf("clicking an issue row left the pane on %v", app.focusedPane)
	}
	if row, _ := app.listIssuesTable.GetSelection(); row != target {
		t.Errorf("selected row = %d, want %d: the press never reached the table", row, target)
	}
}

func TestPaneAtIgnoresAPaneTheLayoutDropped(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 60, 40, FocusNavigation)
	if app.layoutMode != layoutNarrow {
		t.Fatalf("layout mode = %v, want narrow", app.layoutMode)
	}

	app.detailsView.SetRect(0, 0, 60, 40)
	pane, ok := app.paneAt(10, 10)
	if !ok || pane != FocusNavigation {
		t.Errorf("paneAt = %v, %v; want the navigation pane, which is the one on screen", pane, ok)
	}
}

func TestTheMouseCaptureRunsBeforeTheClickIsDelivered(t *testing.T) {
	app := newMouseTestApp(t)
	screen := layOut(t, app, 180, 40, FocusNavigation)

	x, y := paneCenter(t, app.issuesColumn)

	go func() { _ = app.app.Run() }()
	t.Cleanup(func() { app.app.Stop() })

	screen.InjectMouse(x, y, tcell.ButtonPrimary, tcell.ModNone)
	screen.InjectMouse(x, y, tcell.ButtonNone, tcell.ModNone)

	deadline := time.After(4 * time.Second)
	for {
		pane := make(chan FocusTarget, 1)
		go func() { app.app.QueueUpdateDraw(func() { pane <- app.focusedPane }) }()
		select {
		case got := <-pane:
			if got == FocusIssues {
				return
			}
		case <-deadline:
			t.Fatal("the injected click never moved the pane: the capture no longer runs before the primitives")
		}
	}
}

func TestCrossingABreakpointWhileZoomedDoesNotFreezeTheApp(t *testing.T) {
	app := newMouseTestApp(t)
	screen := layOut(t, app, 180, 40, FocusIssues)
	app.toggleDetailsZoom()
	app.focusPane(FocusNavigation)
	if !app.detailsZoomed || app.focusedPane != FocusNavigation {
		t.Fatalf("setup: zoomed = %v, pane = %v", app.detailsZoomed, app.focusedPane)
	}

	screen.SetSize(90, 40)
	app.app.ForceDraw()

	if app.layoutMode != layoutMedium {
		t.Fatalf("layout mode = %v, want medium", app.layoutMode)
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("the pane stayed on %v after the zoom dropped the tree", app.focusedPane)
	}
	if !app.layoutFocusStale {
		t.Fatal("the draw left no repair for the keyboard to follow")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	if app.layoutFocusStale {
		t.Error("the first key after the resize did not repair focus")
	}
	if focus := app.app.GetFocus(); focus == tview.Primitive(app.navigationTree) {
		t.Error("the keyboard is still on the tree the zoom took off screen")
	}
}
