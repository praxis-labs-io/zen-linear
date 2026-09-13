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

func TestSwitchWorkspaceMissingKeyKeepsCurrent(t *testing.T) {
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")

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

func TestSwitchWorkspaceUnknownNameKeepsCurrent(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{Workspaces: switcherTestWorkspaces()})
	app.activeWorkspaceName = "Acme"

	app.switchWorkspace("Nonexistent")

	if app.activeWorkspaceName != "Acme" {
		t.Errorf("activeWorkspaceName = %q after unknown switch, want Acme", app.activeWorkspaceName)
	}
}

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
	app.queueUpdateDraw = func(func()) {}
	app.app.SetRoot(app.pages, true)
	app.activeWorkspaceName = "Acme"
	app.toggleDetailsPane()
	stopBackgroundWorkOnCleanup(t, app)
	return app
}

func TestSwitchWorkspaceKeepsPaneFocus(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(app *App)
		want  func(app *App) tview.Primitive
	}{
		{
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
			name:  "query box",
			setUp: func(app *App) { app.focusNavSearch() },
			want:  func(app *App) tview.Primitive { return app.navigationTree },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newSwitcherFlowTestApp(t)
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

func TestSwitchWorkspaceKeepsIssuesFocusWhenTheNewListLands(t *testing.T) {
	app := newSwitcherReloadTestApp(t)
	navSettled := installNavSettledHook(app)
	refreshDone := installRefreshCompletionHook(app)
	app.focusedPane = FocusIssues
	app.updateFocus()

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

func TestSwitchWorkspaceReloadsOnASharedKey(t *testing.T) {
	app := newSwitcherFlowTestApp(t)
	t.Setenv("TEST_LINEAR_KEY_SIDE", "k-acme")

	before := app.api
	generation := app.resetGeneration.Load()

	app.switchWorkspace("Side")

	if app.activeWorkspaceName != "Side" {
		t.Fatalf("activeWorkspaceName = %q, want %q", app.activeWorkspaceName, "Side")
	}
	if app.api == before {
		t.Error("API client not rebuilt: the switch was treated as an unchanged config")
	}
	if got := app.resetGeneration.Load(); got == generation {
		t.Error("cached state not reset: the outgoing workspace's issues are still in the pane")
	}
}
