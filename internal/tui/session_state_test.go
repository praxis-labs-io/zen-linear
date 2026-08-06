package tui

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/session"
)

// TestNavSelectionMatchesFetchParams pins capture to the fetch. A saved
// selection has to scope the issue list the same way the live node did, so
// the two branch orders must agree for every navigation kind.
func TestNavSelectionMatchesFetchParams(t *testing.T) {
	tests := []struct {
		name string
		node *NavigationNode
		want session.NavSelection
	}{
		{
			name: "nil selection",
			node: nil,
			want: session.NavSelection{Kind: session.NavAll},
		},
		{
			name: "workspace all issues",
			node: &NavigationNode{ID: "all"},
			want: session.NavSelection{Kind: session.NavAll},
		},
		{
			name: "team-scoped all issues favorite",
			node: &NavigationNode{ID: "all", TeamID: "team-1", FavoriteID: "fav-1"},
			want: session.NavSelection{Kind: session.NavAll, TeamID: "team-1", FavoriteID: "fav-1"},
		},
		{
			name: "team",
			node: &NavigationNode{ID: "team-1", TeamID: "team-1", IsTeam: true},
			want: session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"},
		},
		{
			name: "project",
			node: &NavigationNode{ID: "proj-1", TeamID: "team-1", IsProject: true},
			want: session.NavSelection{Kind: session.NavProject, TeamID: "team-1", ProjectID: "proj-1"},
		},
		{
			name: "status",
			node: &NavigationNode{ID: "state-1", TeamID: "team-1", IsStatus: true, StateID: "state-1"},
			want: session.NavSelection{Kind: session.NavStatus, TeamID: "team-1", StateID: "state-1"},
		},
		{
			name: "cycle",
			node: &NavigationNode{ID: "cycle-1", TeamID: "team-1", IsCycle: true, CycleID: "cycle-1"},
			want: session.NavSelection{Kind: session.NavCycle, TeamID: "team-1", CycleID: "cycle-1"},
		},
		{
			name: "custom view",
			node: &NavigationNode{ID: "view-1", CustomViewID: "view-1", FavoriteID: "fav-2"},
			want: session.NavSelection{Kind: session.NavCustomView, CustomViewID: "view-1", FavoriteID: "fav-2"},
		},
		{
			name: "triage predefined view",
			node: &NavigationNode{ID: "fav-3", TeamID: "team-1", StateType: "triage", FavoriteID: "fav-3"},
			want: session.NavSelection{Kind: session.NavStateType, TeamID: "team-1", StateType: "triage", FavoriteID: "fav-3"},
		},
	}

	app := newUXTestApp(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := navSelectionFor(tt.node)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("navSelectionFor() = %+v, want %+v", got, tt.want)
			}

			app.selectedNavigation = tt.node
			params := app.currentFetchParams("")
			if got.TeamID != params.TeamID {
				t.Errorf("TeamID = %q, fetch params use %q", got.TeamID, params.TeamID)
			}
			if got.ProjectID != params.ProjectID {
				t.Errorf("ProjectID = %q, fetch params use %q", got.ProjectID, params.ProjectID)
			}
			if got.StateID != params.StateID {
				t.Errorf("StateID = %q, fetch params use %q", got.StateID, params.StateID)
			}
			if got.CycleID != params.CycleID {
				t.Errorf("CycleID = %q, fetch params use %q", got.CycleID, params.CycleID)
			}
			if got.CustomViewID != params.CustomViewID {
				t.Errorf("CustomViewID = %q, fetch params use %q", got.CustomViewID, params.CustomViewID)
			}
			if got.StateType != params.StateType {
				t.Errorf("StateType = %q, fetch params use %q", got.StateType, params.StateType)
			}
		})
	}
}

// TestSessionFiltersRoundTrip verifies filters survive storage in both
// directions, names included so the status bar keeps reading in words.
func TestSessionFiltersRoundTrip(t *testing.T) {
	estimate := 5.0
	want := IssueFilters{
		AssigneeID:   "user-1",
		AssigneeName: "Drew",
		LabelIDs:     []string{"label-1"},
		LabelNames:   []string{"bug"},
		StateID:      "state-1",
		StateName:    "In Progress",
		ProjectID:    "proj-1",
		ProjectName:  "Website",
		CycleID:      "cycle-1",
		CycleName:    "Cycle 3",
		DueDate:      linearapi.DateFilter{Eq: "2026-08-06"},
		Estimate:     linearapi.NumberFilter{Eq: &estimate},
	}

	got := filtersFromSession(sessionFiltersFor(want))
	if got.Summary() != want.Summary() {
		t.Fatalf("Summary() = %q, want %q", got.Summary(), want.Summary())
	}
	if got.Estimate.Eq == nil || *got.Estimate.Eq != estimate {
		t.Fatalf("Estimate = %+v, want %v", got.Estimate.Eq, estimate)
	}
	if got.DueDate.Eq != want.DueDate.Eq {
		t.Fatalf("DueDate = %q, want %q", got.DueDate.Eq, want.DueDate.Eq)
	}
}

// TestSessionFiltersEmptyRoundTrip verifies no filters stays no filters, so a
// restore does not resurrect an empty date or estimate filter.
func TestSessionFiltersEmptyRoundTrip(t *testing.T) {
	if got := filtersFromSession(sessionFiltersFor(IssueFilters{})); !got.Empty() {
		t.Fatalf("filtersFromSession(empty) = %+v, want empty", got)
	}
}

// TestPersistSessionKeepsOtherWorkspaces verifies recording one workspace does
// not wipe the place saved for another.
func TestPersistSessionKeepsOtherWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	other := session.State{Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-9"}}
	if err := session.Record(path, "Beta", other); err != nil {
		t.Fatalf("seed Record: %v", err)
	}

	app := newUXTestApp(t)
	app.config.SessionRestore = true
	app.sessionPath = path
	app.activeWorkspaceName = "Alpha"
	app.selectedNavigation = &NavigationNode{ID: "proj-1", TeamID: "team-1", IsProject: true}

	app.persistSession()

	file, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if file.LastWorkspace != "Alpha" {
		t.Fatalf("LastWorkspace = %q, want Alpha", file.LastWorkspace)
	}
	if got, ok := file.StateFor("Beta"); !ok || got.Nav.TeamID != "team-9" {
		t.Fatalf("Beta state = %+v, %v, want team-9", got, ok)
	}
	got, ok := file.StateFor("Alpha")
	if !ok {
		t.Fatal("Alpha state missing")
	}
	if got.Nav.Kind != session.NavProject || got.Nav.ProjectID != "proj-1" {
		t.Fatalf("Alpha nav = %+v, want project proj-1", got.Nav)
	}
}

// TestPersistSessionSkipped verifies no file appears when there is nowhere to
// write or the user turned restore off.
func TestPersistSessionSkipped(t *testing.T) {
	tests := []struct {
		name    string
		path    bool
		restore bool
		nav     *NavigationNode
	}{
		{name: "no path", path: false, restore: true, nav: &NavigationNode{ID: "all"}},
		{name: "restore disabled", path: true, restore: false, nav: &NavigationNode{ID: "all"}},
		{name: "startup never loaded", path: true, restore: true, nav: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			app := newUXTestApp(t)
			app.config.SessionRestore = tt.restore
			app.activeWorkspaceName = "Alpha"
			app.selectedNavigation = tt.nav
			if tt.path {
				app.sessionPath = path
			}

			app.persistSession()

			if file, err := session.Load(path); err != nil || file.LastWorkspace != "" {
				t.Fatalf("session written: file=%+v err=%v", file, err)
			}
		})
	}
}

// TestUseSessionRespectsToggle verifies the restore point is only picked up
// when the user wants it, while the write path stays armed either way.
func TestUseSessionRespectsToggle(t *testing.T) {
	file := session.File{
		LastWorkspace: "Alpha",
		Workspaces: map[string]session.State{
			"alpha": {Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-1"}},
		},
	}

	for _, restore := range []bool{true, false} {
		app := newUXTestApp(t)
		app.config.SessionRestore = restore
		app.activeWorkspaceName = "Alpha"

		app.UseSession("/tmp/session.json", file)

		if app.sessionPath != "/tmp/session.json" {
			t.Errorf("sessionPath = %q, want it kept regardless of the toggle", app.sessionPath)
		}
		if restore && app.pendingSession == nil {
			t.Error("pendingSession = nil, want the saved state")
		}
		if !restore && app.pendingSession != nil {
			t.Errorf("pendingSession = %+v, want nil when restore is off", app.pendingSession)
		}
	}
}

// TestUseSessionIgnoresOtherWorkspaces verifies a record saved for a different
// workspace does not restore ids that would not resolve here.
func TestUseSessionIgnoresOtherWorkspaces(t *testing.T) {
	app := newUXTestApp(t)
	app.config.SessionRestore = true
	app.activeWorkspaceName = "Alpha"

	app.UseSession("/tmp/session.json", session.File{
		LastWorkspace: "Beta",
		Workspaces: map[string]session.State{
			"beta": {Nav: session.NavSelection{Kind: session.NavTeam, TeamID: "team-9"}},
		},
	})

	if app.pendingSession != nil {
		t.Fatalf("pendingSession = %+v, want nil", app.pendingSession)
	}
}

// TestSwitchWorkspaceRecordsOutgoingSession verifies the place is captured
// before the switch clears it, not left to a quit that never sees it.
func TestSwitchWorkspaceRecordsOutgoingSession(t *testing.T) {
	t.Setenv("LINEAR_KEY_BETA", "key-beta")

	path := filepath.Join(t.TempDir(), "session.json")
	app := newUXTestApp(t)
	app.config.SessionRestore = true
	app.config.Workspaces = []config.Workspace{
		{Name: "Alpha", APIKeyEnv: "LINEAR_KEY_ALPHA"},
		{Name: "Beta", APIKeyEnv: "LINEAR_KEY_BETA"},
	}
	app.sessionPath = path
	app.activeWorkspaceName = "Alpha"
	app.selectedNavigation = &NavigationNode{ID: "state-1", TeamID: "team-1", IsStatus: true, StateID: "state-1"}

	app.switchWorkspace("Beta")

	file, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	state, ok := file.StateFor("Alpha")
	if !ok {
		t.Fatal("outgoing workspace state missing")
	}
	if state.Nav.Kind != session.NavStatus || state.Nav.StateID != "state-1" {
		t.Fatalf("Alpha nav = %+v, want status state-1", state.Nav)
	}
	if file.LastWorkspace != "Beta" {
		t.Fatalf("LastWorkspace = %q, want Beta so a crash before the next quit reopens the workspace switched into", file.LastWorkspace)
	}
}
