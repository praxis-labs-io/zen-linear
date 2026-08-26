package tui

import (
	"context"
	"testing"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/session"
)

// newSessionRestoreTestApp extends the default-navigation harness with the
// states, cycles, and captured fetch params a session restore needs.
func newSessionRestoreTestApp(t *testing.T, state session.State) (*App, *linearapi.FetchIssuesParams) {
	t.Helper()

	app := newDefaultNavTestApp(t, config.Config{SessionRestore: true})
	app.fetchWorkflowStatesFunc = func(context.Context, string) ([]linearapi.WorkflowState, error) {
		return []linearapi.WorkflowState{
			{ID: "state-1", Name: "Todo", Type: "unstarted", Position: 1},
			{ID: "state-2", Name: "In Progress", Type: "started", Position: 2},
		}, nil
	}
	app.fetchCyclesFunc = func(context.Context, string) ([]linearapi.Cycle, error) {
		return []linearapi.Cycle{{ID: "cycle-1", Number: 3, IsActive: true}}, nil
	}

	var captured linearapi.FetchIssuesParams
	app.fetchIssuesPage = func(_ context.Context, params linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		captured = params
		return linearapi.IssuePage{
			Issues: []linearapi.Issue{
				{ID: "issue-1", Identifier: "ENG-1", Title: "First"},
				{ID: "issue-2", Identifier: "ENG-2", Title: "Second"},
			},
		}, nil
	}

	app.pendingSession = &state
	return app, &captured
}

// sessionFavorites returns favorite fixtures covering a custom view nested in
// a folder and a top-level project.
func sessionFavorites() []linearapi.Favorite {
	return []linearapi.Favorite{
		{ID: "fav-folder", Type: "folder", FolderName: "Saved views"},
		{ID: "fav-view", Type: "customView", ParentID: "fav-folder", CustomViewID: "view-1", CustomViewName: "My cycle"},
		{ID: "fav-project", Type: "project", ProjectID: "proj-2", ProjectName: "Mobile App"},
	}
}

// startSessionRestore mirrors loadInitialData: the pending session and the
// fetch seams are snapshotted first, then the resolve runs against them.
func (a *App) startSessionRestore(ctx context.Context, teams []linearapi.Team, favorites []linearapi.Favorite) bool {
	return a.applySessionNavigation(ctx, a.consumePendingSession(), teams, favorites, a.teamChildFetchers())
}

// restoreSession runs a full startup restore against the fixtures and returns
// whether the restore claimed the startup refresh.
func restoreSession(t *testing.T, app *App, favorites []linearapi.Favorite) bool {
	t.Helper()
	refreshDone := installRefreshCompletionHook(app)
	teams := defaultNavTeams()
	app.rebuildNavigationTree(teams, favorites)

	claimed := app.startSessionRestore(context.Background(), teams, favorites)
	if claimed {
		waitForRefreshCompletion(t, refreshDone)
	}
	return claimed
}

func TestApplySessionNavigationSelectsProject(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavProject, TeamID: "team-1", ProjectID: "proj-2"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	nav := currentNavigationNode(t, app)
	if !nav.IsProject || nav.ID != "proj-2" {
		t.Fatalf("current node = %+v, want project proj-2", nav)
	}
	if app.selectedNavigation == nil || app.selectedNavigation.ID != "proj-2" {
		t.Fatalf("selectedNavigation = %+v, want proj-2", app.selectedNavigation)
	}
}

// TestApplySessionNavigationSelectsStatus covers the node kind nested under a
// non-selectable group, which a search of a team's direct children misses.
func TestApplySessionNavigationSelectsStatus(t *testing.T) {
	app, params := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavStatus, TeamID: "team-1", StateID: "state-2"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	nav := currentNavigationNode(t, app)
	if !nav.IsStatus || nav.StateID != "state-2" {
		t.Fatalf("current node = %+v, want status state-2", nav)
	}
	if params.StateID != "state-2" || params.TeamID != "team-1" {
		t.Fatalf("fetch params = %+v, want team-1 scoped to state-2", params)
	}
}

func TestApplySessionNavigationSelectsCycle(t *testing.T) {
	app, params := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavCycle, TeamID: "team-1", CycleID: "cycle-1"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	nav := currentNavigationNode(t, app)
	if !nav.IsCycle || nav.CycleID != "cycle-1" {
		t.Fatalf("current node = %+v, want cycle cycle-1", nav)
	}
	if params.CycleID != "cycle-1" {
		t.Fatalf("fetch params = %+v, want cycle-1", params)
	}
}

func TestApplySessionNavigationSelectsTeam(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-2"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	nav := currentNavigationNode(t, app)
	if !nav.IsTeam || nav.TeamID != "team-2" {
		t.Fatalf("current node = %+v, want team team-2", nav)
	}
}

// TestApplySessionNavigationSelectsFavoriteInFolder verifies a favorite is
// found without a team anchor, folder nesting included.
func TestApplySessionNavigationSelectsFavoriteInFolder(t *testing.T) {
	app, params := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavCustomView, CustomViewID: "view-1", FavoriteID: "fav-view"},
	})

	if !restoreSession(t, app, sessionFavorites()) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	nav := currentNavigationNode(t, app)
	if nav.CustomViewID != "view-1" {
		t.Fatalf("current node = %+v, want custom view view-1", nav)
	}
	if params.CustomViewID != "view-1" {
		t.Fatalf("fetch params = %+v, want custom view view-1", params)
	}
}

// TestApplySessionNavigationRestoresFiltersBeforeFetch pins the ordering:
// the fetch reads the filters the moment it starts, so restoring them after
// the refresh call would send an unfiltered query.
func TestApplySessionNavigationRestoresFiltersBeforeFetch(t *testing.T) {
	app, params := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
		Filters: session.Filters{
			AssigneeID:   "user-1",
			AssigneeName: "Drew",
			LabelIDs:     []string{"label-1"},
			LabelNames:   []string{"bug"},
		},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	if params.AssigneeID != "user-1" {
		t.Fatalf("fetch params assignee = %q, want user-1", params.AssigneeID)
	}
	if len(params.LabelIDs) != 1 || params.LabelIDs[0] != "label-1" {
		t.Fatalf("fetch params labels = %v, want [label-1]", params.LabelIDs)
	}
	if app.richFilters.AssigneeName != "Drew" {
		t.Errorf("richFilters assignee name = %q, want Drew for the status bar", app.richFilters.AssigneeName)
	}
}

func TestApplySessionNavigationRestoresFocusedIssue(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav:     session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
		IssueID: "issue-2",
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	if got := app.selectedIssueID(IssuesSectionList); got != "issue-2" {
		t.Fatalf("selected issue = %q, want issue-2", got)
	}
}

// TestApplySessionNavigationMissingIssueFallsBack verifies a deleted issue
// leaves the user on the first row rather than nothing.
func TestApplySessionNavigationMissingIssueFallsBack(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav:     session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
		IssueID: "issue-gone",
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	if got := app.selectedIssueID(IssuesSectionList); got != "issue-1" {
		t.Fatalf("selected issue = %q, want the first row issue-1", got)
	}
}

func TestApplySessionNavigationRestoresSearch(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav:    session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
		Search: "login",
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	if got := app.navSearchInput.GetText(); got != "login" {
		t.Fatalf("search input = %q, want login", got)
	}
	if app.searchQuery != "login" {
		t.Fatalf("searchQuery = %q, want login", app.searchQuery)
	}
	// The restored query runs through the debounce, and the section follows the
	// results rather than being restored on its own.
	waitForSearchRows(t, app, 2)
	if app.activeIssuesSection != IssuesSectionSearch {
		t.Fatalf("activeIssuesSection = %v, want Search", app.activeIssuesSection)
	}
}

// TestApplySessionNavigationSelectsSavedSearchIssue verifies a restored search
// reopens on the issue it was left on. updateIssuesData returns early while
// results are showing, so the selection can only happen once they land.
func TestApplySessionNavigationSelectsSavedSearchIssue(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav:     session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
		Search:  "login",
		IssueID: "issue-2",
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}
	waitForSearchRows(t, app, 2)

	if got := app.selectedIssueID(IssuesSectionSearch); got != "issue-2" {
		t.Fatalf("selected search issue = %q, want issue-2 (the row the user left on, not the first result)", got)
	}
}

// TestApplySessionNavigationSkipsSearchWithNoSavedQuery verifies a session left
// on the list opens on the list, and spends no API call on a search nobody
// asked for.
func TestApplySessionNavigationSkipsSearchWithNoSavedQuery(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	if got := app.navSearchInput.GetText(); got != "" {
		t.Fatalf("search input = %q, want empty", got)
	}
	if app.activeIssuesSection != IssuesSectionList {
		t.Fatalf("activeIssuesSection = %v, want the list", app.activeIssuesSection)
	}
}

// TestApplySessionNavigationKeepsNavigationFocus verifies the restore never
// pulls focus out of the navigation pane, with or without a saved query.
func TestApplySessionNavigationKeepsNavigationFocus(t *testing.T) {
	for name, query := range map[string]string{"list": "", "search": "login"} {
		t.Run(name, func(t *testing.T) {
			app, _ := newSessionRestoreTestApp(t, session.State{
				Nav:    session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
				Search: query,
			})

			if !restoreSession(t, app, nil) {
				t.Fatal("applySessionNavigation() = false, want true")
			}
			if query != "" {
				// The restored query runs through the debounce; let it land
				// rather than leaving its goroutine to outlive the test.
				waitForSearchRows(t, app, 2)
			}

			if app.focusedPane != FocusNavigation {
				t.Fatalf("focusedPane = %v, want FocusNavigation", app.focusedPane)
			}
		})
	}
}

// TestApplySessionNavigationMissingTargets verifies every unresolvable saved
// selection hands startup back to the configured default, quietly.
func TestApplySessionNavigationMissingTargets(t *testing.T) {
	tests := []struct {
		name string
		nav  session.NavSelection
	}{
		{name: "deleted team", nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-gone"}},
		{name: "deleted project", nav: session.NavSelection{Kind: session.NavProject, TeamID: "team-1", ProjectID: "proj-gone"}},
		{name: "deleted status", nav: session.NavSelection{Kind: session.NavStatus, TeamID: "team-1", StateID: "state-gone"}},
		{name: "deleted cycle", nav: session.NavSelection{Kind: session.NavCycle, TeamID: "team-1", CycleID: "cycle-gone"}},
		{name: "unfavorited view", nav: session.NavSelection{Kind: session.NavCustomView, CustomViewID: "view-1", FavoriteID: "fav-gone"}},
		{name: "kind from a newer build", nav: session.NavSelection{Kind: "constellation", TeamID: "team-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newSessionRestoreTestApp(t, session.State{Nav: tt.nav})

			if restoreSession(t, app, sessionFavorites()) {
				t.Fatal("applySessionNavigation() = true, want false so the configured default runs")
			}
			if app.statusMessage != "" {
				t.Errorf("statusMessage = %q, want no flash for machine state", app.statusMessage)
			}
		})
	}
}

// TestApplySessionNavigationIsOneShot verifies a settings save or workspace
// switch, both of which re-run loadInitialData, does not reapply a stale
// place over where the user actually is.
func TestApplySessionNavigationIsOneShot(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("first applySessionNavigation() = false, want true")
	}
	if app.startSessionRestore(context.Background(), defaultNavTeams(), nil) {
		t.Fatal("second applySessionNavigation() = true, want false")
	}
}

// TestApplySessionNavigationWithoutPendingState verifies an app that never
// had a session to restore leaves startup to the configured default.
func TestApplySessionNavigationWithoutPendingState(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{SessionRestore: true})

	if app.startSessionRestore(context.Background(), defaultNavTeams(), nil) {
		t.Fatal("applySessionNavigation() = true, want false")
	}
}

// A restore that needs no team children must not build the team's rows out of
// the empty slices it never fetched. That left the team holding its own All
// Issues row and nothing else, with no open able to go back for the rest.
func TestRestoringATeamLeavesItsRowsToBeLoaded(t *testing.T) {
	app, _ := newSessionRestoreTestApp(t, session.State{
		Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
	})

	if !restoreSession(t, app, nil) {
		t.Fatal("applySessionNavigation() = false, want true")
	}

	teamNode := app.findTeamTreeNode("team-1")
	if teamChildrenLoaded(teamNode) {
		t.Fatal("a restore that fetched nothing claimed the team's rows were built")
	}
	if got := len(teamNode.GetChildren()); got != 0 {
		t.Fatalf("team rows after the restore = %d, want none until it is opened", got)
	}
}
