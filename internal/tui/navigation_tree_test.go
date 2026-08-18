package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

// pressEnterOnNavigation drives the real tree input handler, so the test
// exercises the selection path a keypress takes rather than calling the
// callback directly.
func pressEnterOnNavigation(app *App) {
	handler := app.navigationTree.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
}

// TestSelectingNavigationItemKeepsFocusInTheNavigationPane guards the behavior
// Drew asked for: Enter picks a view without throwing focus at the issues list.
func TestSelectingNavigationItemKeepsFocusInTheNavigationPane(t *testing.T) {
	app := newUXTestApp(t)
	refreshed := make(chan struct{}, 1)
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{}, nil
	}
	app.refreshCompleted = func() { refreshed <- struct{}{} }

	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.focusedPane = FocusNavigation
	app.navigationTree.SetCurrentNode(app.navigationTree.GetRoot().GetChildren()[0])

	pressEnterOnNavigation(app)

	if app.focusedPane != FocusNavigation {
		t.Fatalf("focusedPane = %v, want FocusNavigation", app.focusedPane)
	}
	if app.selectedNavigation == nil || app.selectedNavigation.ID != "all" {
		t.Fatalf("selectedNavigation = %+v, want All Issues", app.selectedNavigation)
	}

	select {
	case <-refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("selecting a navigation item did not refresh the issues list")
	}
}

// TestSelectingFavoritesFolderOnlyToggles verifies a folder expands instead of
// filtering.
func TestSelectingFavoritesFolderOnlyToggles(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		t.Error("selecting a folder must not refresh issues")
		return linearapi.IssuePage{}, nil
	}

	favorites := []linearapi.Favorite{
		{ID: "folder-1", Type: "folder", FolderName: "Work", SortOrder: 1},
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", ParentID: "folder-1", SortOrder: 2},
	}
	app.rebuildNavigationTree(nil, favorites)

	folderNode := app.favoritesGroup.GetChildren()[0]
	app.navigationTree.SetCurrentNode(folderNode)
	if !folderNode.IsExpanded() {
		t.Fatal("folder should start expanded")
	}

	pressEnterOnNavigation(app)

	if folderNode.IsExpanded() {
		t.Error("Enter on a favorites folder did not collapse it")
	}
}

// TestNavigationPaneDrawsItsControlsInOrder covers the pane's stack: the
// workspace names the pane border, the query box sits in a frame of its own
// under it, and the tree starts one column off the border with no root row
// above it.
func TestNavigationPaneDrawsItsControlsInOrder(t *testing.T) {
	app := newUXTestApp(t)
	app.activeWorkspaceName = "Praxis Labs"
	app.updateAllPaneTitles()
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)

	lines := drawPrimitive(t, app.navigationPanel, 40)

	if !strings.Contains(lines[0], "Praxis Labs") {
		t.Errorf("pane title row = %q, want the workspace name", lines[0])
	}
	if got := lines[1]; !strings.Contains(got, "┌") {
		t.Errorf("first row = %q, want the query box's own frame", got)
	}
	if got := lines[2]; !strings.Contains(got, "/") || !strings.Contains(got, "Search") {
		t.Errorf("second row = %q, want the query box", got)
	}
	if got := lines[3]; !strings.Contains(got, "└") {
		t.Errorf("third row = %q, want the frame closing under the query box", got)
	}
	if got := lines[4]; !strings.HasPrefix(strings.TrimPrefix(got, "│"), " All Issues") {
		t.Errorf("first tree row = %q, want All Issues one column off the border", got)
	}
}
