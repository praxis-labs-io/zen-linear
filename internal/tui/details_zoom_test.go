package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func zoomKey(app *App) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'v', tcell.ModNone))
}

func newZoomTestApp(t *testing.T) *App {
	t.Helper()

	app := newUXTestApp(t)
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "ZNL-35", Title: "Focused details view"}
	return app
}

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

func TestUnzoomReturnsToThePaneTheZoomCameFrom(t *testing.T) {
	for _, from := range []FocusTarget{FocusNavigation, FocusIssues, FocusDetails} {
		app := newZoomTestApp(t)
		app.detailsHidden = false
		app.layoutMode = layoutWide
		app.focusedPane = from

		zoomKey(app)
		if app.focusedPane != FocusDetails {
			t.Errorf("from %v: focusedPane = %v while zoomed, want FocusDetails", from, app.focusedPane)
		}

		zoomKey(app)
		if app.detailsZoomed {
			t.Fatalf("from %v: the second v did not unzoom", from)
		}
		if app.focusedPane != from {
			t.Errorf("from %v: focusedPane = %v after unzooming, want %v", from, app.focusedPane, from)
		}
		if app.detailsHidden {
			t.Errorf("from %v: unzooming closed the details pane", from)
		}
	}
}

func TestEveryExitFromTheZoomRestoresAClosedDetailsPane(t *testing.T) {
	exits := map[string]func(*App){
		"v again":       zoomKey,
		"pane number 2": func(a *App) { a.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '2', tcell.ModNone)) },
		"escape":        func(a *App) { a.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)) },
		"navigation selection": func(a *App) {
			a.onNavigationSelected(&NavigationNode{ID: "team-1", Text: "Engineering", TeamID: "team-1", IsTeam: true})
		},
		"workspace reset": (*App).resetCachedState,
	}

	for name, exit := range exits {
		t.Run(name, func(t *testing.T) {
			app := newZoomTestApp(t)
			app.layoutMode = layoutWide
			app.focusedPane = FocusIssues
			if !app.detailsHidden {
				t.Fatal("details pane starts open; this test covers the closed case")
			}

			zoomKey(app)
			if !app.detailsZoomed || app.detailsHidden {
				t.Fatal("the zoom did not open the details pane")
			}

			exit(app)

			if app.detailsZoomed {
				t.Errorf("%s left the zoom on", name)
			}
			if !app.detailsHidden {
				t.Errorf("%s left the details rail on screen, when it was closed before the zoom", name)
			}
			if !mountsIssuesColumn(app) {
				t.Errorf("%s left the issues column off screen", name)
			}
		})
	}
}

func TestUnzoomRestoresAClosedDetailsPane(t *testing.T) {
	app := newZoomTestApp(t)
	app.focusedPane = FocusIssues
	if !app.detailsHidden {
		t.Fatal("details pane starts open; this test covers the closed case")
	}

	zoomKey(app)
	if app.detailsHidden {
		t.Fatal("zoom did not open the details pane")
	}

	zoomKey(app)
	if !app.detailsHidden {
		t.Error("unzooming left the details pane open, when it was closed before the zoom")
	}
	if app.focusedPane != FocusIssues {
		t.Errorf("focusedPane = %v, want FocusIssues", app.focusedPane)
	}
}

func contentPanes(app *App) []tview.Primitive {
	panes := make([]tview.Primitive, 0, app.contentFlex.GetItemCount())
	for i := 0; i < app.contentFlex.GetItemCount(); i++ {
		panes = append(panes, app.contentFlex.GetItem(i))
	}
	return panes
}

func mountsIssuesColumn(app *App) bool {
	for _, pane := range contentPanes(app) {
		if pane == app.issuesColumn {
			return true
		}
	}
	return false
}

func TestSelectingANavigationNodeBringsTheIssuesListBack(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.layoutMode = layoutWide
	app.focusedPane = FocusNavigation
	zoomKey(app)
	if mountsIssuesColumn(app) {
		t.Fatal("the zoom did not take the issues column off screen")
	}

	app.onNavigationSelected(&NavigationNode{ID: "team-1", Text: "Engineering", TeamID: "team-1", IsTeam: true})

	if app.detailsZoomed {
		t.Error("selecting a navigation node left the zoom on")
	}
	if !mountsIssuesColumn(app) {
		t.Error("the issues column is still off screen after selecting a list")
	}
}

func TestZoomNeedsASelectedIssue(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues

	zoomKey(app)
	if app.detailsZoomed {
		t.Error("zoomed with no issue selected")
	}
}

func TestStepSkipsTheIssuesPaneWhileZoomed(t *testing.T) {
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

	stepKey(app, 'h')
	if app.focusedPane == FocusIssues {
		t.Error("h landed on the issues pane while zoomed")
	}
}

func TestStepHoldsTheDetailsPaneWhileZoomedAndNarrow(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true
	app.layoutMode = layoutNarrow

	stepKey(app, 'h')
	if app.focusedPane != FocusDetails {
		t.Errorf("focusedPane = %v, want the zoom to hold FocusDetails", app.focusedPane)
	}
}

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

func TestLeftMovesToTheNavigationPaneWhileZoomed(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true
	app.layoutMode = layoutWide

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))

	if app.focusedPane != FocusNavigation {
		t.Errorf("focusedPane = %v, want FocusNavigation", app.focusedPane)
	}
	if !app.detailsZoomed {
		t.Error("Left released the zoom instead of moving focus")
	}
}

func TestRightReturnsToTheZoomedDetailsPane(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusNavigation
	app.detailsZoomed = true
	app.layoutMode = layoutWide

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))

	if app.focusedPane != FocusDetails {
		t.Errorf("focusedPane = %v, want FocusDetails", app.focusedPane)
	}
}

func TestLeftHoldsTheZoomedPaneWithNoNavOnScreen(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsZoomed = true
	app.layoutMode = layoutNarrow

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))

	if app.focusedPane != FocusDetails {
		t.Errorf("focusedPane = %v, want FocusDetails", app.focusedPane)
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

func TestCtrlDAndCtrlUScrollTheDetailsPaneHalfAPage(t *testing.T) {
	const height = 40
	app := newDetailsTestApp(t)
	app.focusedPane = FocusDetails
	drawDetails(t, app, 60)
	app.detailsPageView.SetRect(0, 0, 60, height)

	inner := viewHeight(app.detailsPageView)
	want := inner / 2
	if want < 1 {
		t.Fatalf("inner height %d leaves no half page to scroll", inner)
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))
	if row, _ := app.detailsPageView.GetScrollOffset(); row != want {
		t.Errorf("Ctrl+D scrolled to row %d, want %d", row, want)
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone))
	if row, _ := app.detailsPageView.GetScrollOffset(); row != 0 {
		t.Errorf("Ctrl+U scrolled to row %d, want back to 0", row)
	}
}

func TestCtrlUStopsAtTheTop(t *testing.T) {
	app := newDetailsTestApp(t)
	app.focusedPane = FocusDetails
	drawDetails(t, app, 60)

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone))
	if row, _ := app.detailsPageView.GetScrollOffset(); row != 0 {
		t.Errorf("Ctrl+U at the top scrolled to row %d, want 0", row)
	}
}

func TestShrinkingBelowWideMovesFocusOffTheDroppedNavPane(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.detailsZoomed = true
	app.layoutMode = layoutWide
	app.focusedPane = FocusNavigation

	app.watchLayoutWidth(80)

	if app.layoutMode != layoutMedium {
		t.Fatalf("layoutMode = %v, want layoutMedium", app.layoutMode)
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("focusedPane = %v, want FocusDetails once the tree is gone", app.focusedPane)
	}
}

func TestResetCachedStateReleasesTheZoom(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.detailsZoomed = true

	app.resetCachedState()

	if app.detailsZoomed {
		t.Error("resetCachedState left the zoom on")
	}
}

func TestPaneNumberOneReachesTheNavigationTreeWhileZoomed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       layoutMode
		wantZoomed bool
	}{
		{name: "wide keeps the zoom", mode: layoutWide, wantZoomed: true},
		{name: "medium releases it", mode: layoutMedium, wantZoomed: false},
		{name: "narrow releases it", mode: layoutNarrow, wantZoomed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newZoomTestApp(t)
			app.detailsHidden = false
			app.detailsZoomed = true
			app.focusedPane = FocusDetails
			app.layoutMode = tc.mode

			app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModNone))

			if app.focusedPane != FocusNavigation {
				t.Errorf("focusedPane = %v, want FocusNavigation", app.focusedPane)
			}
			if app.detailsZoomed != tc.wantZoomed {
				t.Errorf("detailsZoomed = %v, want %v", app.detailsZoomed, tc.wantZoomed)
			}
		})
	}
}

func TestZoomedStatusBarHelpMatchesTheKeysThatWork(t *testing.T) {
	app := newZoomTestApp(t)
	app.detailsHidden = false
	app.detailsZoomed = true
	app.focusedPane = FocusDetails

	app.layoutMode = layoutWide
	app.updateStatusBar()
	wide := app.statusBar.GetText(true)
	if !strings.Contains(wide, "navigation") {
		t.Errorf("wide zoomed help = %q, want the navigation key offered", wide)
	}

	app.layoutMode = layoutNarrow
	app.updateStatusBar()
	narrow := app.statusBar.GetText(true)
	if strings.Contains(narrow, "navigation") {
		t.Errorf("narrow zoomed help = %q, want no navigation key with no tree on screen", narrow)
	}
	for _, want := range []string{"v close", "Esc back to list"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("zoomed help = %q, want it to mention %q", narrow, want)
		}
	}
}

func TestReleasingTheZoomFromTheNavSpineMovesFocusOffTheClosedPane(t *testing.T) {
	app := newZoomTestApp(t)
	app.layoutMode = layoutWide
	app.focusedPane = FocusIssues
	zoomKey(app)
	app.focusedPane = FocusDetails

	app.onNavigationSelected(&NavigationNode{ID: "team-1", Text: "Engineering", TeamID: "team-1", IsTeam: true})

	if !app.detailsHidden {
		t.Fatal("the zoom did not restore the closed details pane")
	}
	if app.focusedPane == FocusDetails {
		t.Error("focus is still on the details pane after it left the screen")
	}
}

func TestSelectingAFavoritedIssueKeepsTheZoom(t *testing.T) {
	app := newZoomTestApp(t)
	app.layoutMode = layoutWide
	app.focusedPane = FocusIssues
	zoomKey(app)

	app.onNavigationSelected(&NavigationNode{
		ID: "issue-9", Text: "ZNL-9", TeamID: "team-1", IssueID: "issue-9", IsIssue: true,
	})

	if !app.detailsZoomed {
		t.Error("selecting a favorited issue released the zoom it was going to be read in")
	}
	if app.detailsHidden {
		t.Error("selecting a favorited issue closed the details pane")
	}
}
