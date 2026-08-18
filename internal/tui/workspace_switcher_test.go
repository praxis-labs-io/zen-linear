package tui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func switcherTestWorkspaces() []config.Workspace {
	return []config.Workspace{
		{Name: "Acme", APIKeyEnv: "TEST_LINEAR_KEY_ACME"},
		{Name: "Side", APIKeyEnv: "TEST_LINEAR_KEY_SIDE"},
	}
}

// TestWorkspaceNameForKey verifies the active workspace is identified from the
// resolved token, and that OAuth/explicit tokens map to no workspace.
func TestWorkspaceNameForKey(t *testing.T) {
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")
	t.Setenv("TEST_LINEAR_KEY_SIDE", "k-side")
	workspaces := switcherTestWorkspaces()

	if got := workspaceNameForKey(workspaces, "k-side"); got != "Side" {
		t.Errorf("workspaceNameForKey(k-side) = %q, want Side", got)
	}
	if got := workspaceNameForKey(workspaces, "some-oauth-value"); got != "" {
		t.Errorf("workspaceNameForKey(unmatched) = %q, want empty", got)
	}
	if got := workspaceNameForKey(workspaces, ""); got != "" {
		t.Errorf("workspaceNameForKey(empty) = %q, want empty", got)
	}
}

// TestWorkspacePickerItemsMarksActive verifies picker labels flag the active
// workspace.
func TestWorkspacePickerItemsMarksActive(t *testing.T) {
	items := workspacePickerItems(switcherTestWorkspaces(), "Side")
	if len(items) != 2 {
		t.Fatalf("workspacePickerItems() returned %d items, want 2", len(items))
	}
	if items[0].Label != "Acme" || items[0].ID != "Acme" {
		t.Errorf("items[0] = %+v, want plain Acme entry", items[0])
	}
	if items[1].Label != "Side (active)" || items[1].ID != "Side" {
		t.Errorf("items[1] = %+v, want Side marked active", items[1])
	}
}

// TestSwitchWorkspaceMissingKeyKeepsCurrent verifies switching to a workspace
// whose env var is unset fails without touching the active workspace.
func TestSwitchWorkspaceMissingKeyKeepsCurrent(t *testing.T) {
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")
	// TEST_LINEAR_KEY_SIDE deliberately not set.

	cfg := config.Config{Workspaces: switcherTestWorkspaces()}
	cfg.LinearAPIKey = "k-acme"
	app := newDefaultNavTestApp(t, cfg)
	app.activeWorkspaceName = "Acme"

	app.switchWorkspace("Side")

	if app.activeWorkspaceName != "Acme" {
		t.Errorf("activeWorkspaceName = %q after failed switch, want Acme", app.activeWorkspaceName)
	}
	if app.config.LinearAPIKey != "k-acme" {
		t.Errorf("LinearAPIKey changed after failed switch")
	}
}

// TestSwitchWorkspaceUnknownNameKeepsCurrent verifies an unknown workspace
// name is rejected.
func TestSwitchWorkspaceUnknownNameKeepsCurrent(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{Workspaces: switcherTestWorkspaces()})
	app.activeWorkspaceName = "Acme"

	app.switchWorkspace("Nonexistent")

	if app.activeWorkspaceName != "Acme" {
		t.Errorf("activeWorkspaceName = %q after unknown switch, want Acme", app.activeWorkspaceName)
	}
}

// newSwitcherFlowTestApp builds an app on Acme that can complete a switch to
// Side. SetRoot is what primes the delegate tview moves focus between pages
// with, so a harness without it cannot see focus leave a pane.
func newSwitcherFlowTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")
	t.Setenv("TEST_LINEAR_KEY_SIDE", "k-side")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(server.Close)

	cfg := config.Config{Workspaces: switcherTestWorkspaces(), APIEndpoint: server.URL}
	cfg.LinearAPIKey = "k-acme"
	app := newDefaultNavTestApp(t, cfg)
	// The reload runs off the event loop, which no test app runs. Dropping its
	// queued updates keeps them off the state under assertion.
	app.queueUpdateDraw = func(func()) {}
	app.app.SetRoot(app.pages, true)
	app.activeWorkspaceName = "Acme"
	app.toggleDetailsPane()
	stopBackgroundWorkOnCleanup(t, app)
	return app
}

// TestSwitchWorkspaceKeepsPaneFocus covers focus surviving the reload. The
// modal rebuilds re-add their pages, and tview hands focus from an added page
// down to the pane the layout was built focused on, so the pane the user was
// in went dead until an arrow key put it back.
func TestSwitchWorkspaceKeepsPaneFocus(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(app *App)
		want  func(app *App) tview.Primitive
	}{
		{
			// The switch clears the list, so the placeholder is what the issues
			// column is showing and what focus has to land on. Focusing the
			// detached table would leave no pane looking focused at all.
			name:  "issues",
			setUp: func(app *App) { app.focusedPane = FocusIssues },
			want:  func(app *App) tview.Primitive { return app.issuesPlaceholder },
		},
		{
			name:  "details",
			setUp: func(app *App) { app.focusedPane = FocusDetails },
			want:  func(app *App) tview.Primitive { return app.detailsPageView },
		},
		{
			// The reset empties the query underneath the switch, so leaving
			// focus in the box would park the keyboard on a search the user no
			// longer has. The tree beside it is where the pane belongs.
			name:  "query box",
			setUp: func(app *App) { app.focusNavSearch() },
			want:  func(app *App) tview.Primitive { return app.navigationTree },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newSwitcherFlowTestApp(t)
			// An empty details pane refocuses itself on its way to the empty
			// state, which would hide the bug in the details case.
			app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
			app.updateDetailsView()
			tc.setUp(app)
			app.updateFocus()

			app.switchWorkspace("Side")

			if app.activeWorkspaceName != "Side" {
				t.Fatalf("activeWorkspaceName = %q, want Side", app.activeWorkspaceName)
			}
			if got := app.app.GetFocus(); got != tc.want(app) {
				t.Fatalf("keyboard focus landed on %T, want the %s pane", got, tc.name)
			}
		})
	}
}

// newSwitcherReloadTestApp builds an app whose switch runs all the way through
// to the new workspace's issues. The reload has to go over HTTP: applySettings
// rebuilds the client, which drops any stubbed fetcher with it, so a canned
// response is the only seam that survives the switch. Every query gets the same
// body; the ones that are not the issue list read it as empty.
func newSwitcherReloadTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")
	t.Setenv("TEST_LINEAR_KEY_SIDE", "k-side")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"issue-9","identifier":"SIDE-9","title":"Ninth"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`))
	}))
	t.Cleanup(server.Close)

	cfg := config.Config{Workspaces: switcherTestWorkspaces(), APIEndpoint: server.URL}
	cfg.LinearAPIKey = "k-acme"
	app := newDefaultNavTestApp(t, cfg)
	app.fetchIssuesPage = nil
	app.app.SetRoot(app.pages, true)
	app.activeWorkspaceName = "Acme"
	stopBackgroundWorkOnCleanup(t, app)
	return app
}

// TestSwitchWorkspaceKeepsIssuesFocusWhenTheNewListLands covers the second half
// of the switch. The reset empties the pane, so focus lands on the placeholder,
// and the arriving rows put the table back underneath it.
func TestSwitchWorkspaceKeepsIssuesFocusWhenTheNewListLands(t *testing.T) {
	app := newSwitcherReloadTestApp(t)
	navSettled := installNavSettledHook(app)
	refreshDone := installRefreshCompletionHook(app)
	app.focusedPane = FocusIssues
	app.updateFocus()

	// The picker callback runs on the event loop, and so does the reload it
	// starts. Driving the switch bare would put the two on different goroutines.
	app.QueueUpdateDraw(func() { app.switchWorkspace("Side") })
	waitForNavSettled(t, navSettled)
	waitForRefreshCompletion(t, refreshDone)

	if len(app.listIssueRows) == 0 {
		t.Fatal("the new workspace's issues never landed")
	}
	if got := app.app.GetFocus(); got != tview.Primitive(app.listIssuesTable) {
		t.Fatalf("keyboard focus landed on %T once the new list arrived, want the issues table", got)
	}
}

// TestSwitchWorkspaceEmptiesTheDetailsPane covers the pane still painting an
// issue from the workspace being left, which no command could act on.
func TestSwitchWorkspaceEmptiesTheDetailsPane(t *testing.T) {
	app := newSwitcherFlowTestApp(t)
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app.updateDetailsView()
	app.focusedPane = FocusIssues
	app.updateFocus()

	app.switchWorkspace("Side")

	if got := app.detailsPageView.GetText(true); strings.Contains(got, "Alpha") {
		t.Errorf("details pane still shows the old workspace's issue: %q", got)
	}
	if app.GetSelectedIssue() != nil {
		t.Error("GetSelectedIssue() still returns an issue after the switch")
	}
}
