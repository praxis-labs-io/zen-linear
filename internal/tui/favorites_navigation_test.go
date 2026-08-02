package tui

import (
	"testing"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// TestFavoriteNavigationNodesMapsSupportedTypes verifies issue, project,
// cycle, and team favorites map onto navigation nodes.
func TestFavoriteNavigationNodesMapsSupportedTypes(t *testing.T) {
	favorites := []linearapi.Favorite{
		{
			Type:            "issue",
			IssueID:         "issue-1",
			IssueIdentifier: "ENG-42",
			IssueTitle:      "Fix login",
			IssueTeamID:     "team-1",
		},
		{
			Type:          "project",
			ProjectID:     "project-1",
			ProjectName:   "Website",
			ProjectTeamID: "team-1",
		},
		{
			Type:        "cycle",
			CycleID:     "cycle-1",
			CycleNumber: 7,
			CycleTeamID: "team-2",
		},
		{
			Type:     "team",
			TeamID:   "team-3",
			TeamName: "Platform",
		},
	}

	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) != 4 {
		t.Fatalf("favoriteNavigationNodes() returned %d nodes, want 4", len(nodes))
	}

	issue := nodes[0]
	if !issue.IsIssue || issue.IssueID != "issue-1" || issue.TeamID != "team-1" {
		t.Errorf("issue node = %+v, want IsIssue with IssueID issue-1 TeamID team-1", issue)
	}
	if issue.Text != "ENG-42 Fix login" {
		t.Errorf("issue node text = %q, want %q", issue.Text, "ENG-42 Fix login")
	}

	project := nodes[1]
	if !project.IsProject || project.ID != "project-1" || project.TeamID != "team-1" {
		t.Errorf("project node = %+v, want IsProject with ID project-1 TeamID team-1", project)
	}

	cycle := nodes[2]
	if !cycle.IsCycle || cycle.CycleID != "cycle-1" || cycle.TeamID != "team-2" {
		t.Errorf("cycle node = %+v, want IsCycle with CycleID cycle-1 TeamID team-2", cycle)
	}
	if cycle.Text != "Cycle 7" {
		t.Errorf("cycle node text = %q, want %q", cycle.Text, "Cycle 7")
	}

	team := nodes[3]
	if !team.IsTeam || team.TeamID != "team-3" {
		t.Errorf("team node = %+v, want IsTeam with TeamID team-3", team)
	}
}

// TestFavoriteNavigationNodesSkipsUnsupportedTypes verifies favorite types the
// tree cannot display are dropped rather than rendered or crashing.
func TestFavoriteNavigationNodesSkipsUnsupportedTypes(t *testing.T) {
	favorites := []linearapi.Favorite{
		{Type: "customView", ID: "fav-1"},
		{Type: "label", ID: "fav-2"},
		{Type: "document", ID: "fav-3"},
		{Type: "issue", ID: "fav-5"}, // missing IssueID: skipped defensively
		{Type: "project", ProjectID: "project-1", ProjectName: "Website"},
	}

	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) != 1 {
		t.Fatalf("favoriteNavigationNodes() returned %d nodes, want 1", len(nodes))
	}
	if !nodes[0].IsProject || nodes[0].ID != "project-1" {
		t.Errorf("node = %+v, want the project favorite", nodes[0])
	}
}

// TestFavoriteNavigationNodesFolders verifies folder favorites nest their
// children while unfoldered favorites stay at the top level.
func TestFavoriteNavigationNodesFolders(t *testing.T) {
	favorites := []linearapi.Favorite{
		{Type: "project", ProjectID: "project-0", ProjectName: "Loose"},
		{Type: "folder", ID: "folder-1", Title: "Features"},
		{Type: "project", ID: "fav-a", ProjectID: "project-1", ProjectName: "Inside", ParentID: "folder-1"},
		{Type: "project", ID: "fav-b", ProjectID: "project-2", ProjectName: "Also Inside", ParentID: "folder-1"},
	}

	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) != 2 {
		t.Fatalf("favoriteNavigationNodes() returned %d roots, want 2: %+v", len(nodes), nodes)
	}
	if nodes[0].Text != "Loose" {
		t.Errorf("nodes[0] = %+v, want the loose project first", nodes[0])
	}
	folder := nodes[1]
	if !folder.IsFolder || folder.Text != "Features" {
		t.Fatalf("nodes[1] = %+v, want the Features folder", folder)
	}
	if len(folder.Children) != 2 || folder.Children[0].Text != "Inside" || folder.Children[1].Text != "Also Inside" {
		t.Errorf("folder children = %+v", folder.Children)
	}
	if folder.FavoriteID != "folder-1" || folder.FavoriteParentID != "" {
		t.Errorf("folder identity = %q/%q, want folder-1 at the top level", folder.FavoriteID, folder.FavoriteParentID)
	}
	if folder.Children[0].FavoriteID != "fav-a" || folder.Children[0].FavoriteParentID != "folder-1" {
		t.Errorf("child identity = %q/%q, want fav-a inside folder-1",
			folder.Children[0].FavoriteID, folder.Children[0].FavoriteParentID)
	}
}

// TestFavoriteNavigationNodesOrphanRendersAtTopLevel verifies a favorite whose
// folder is absent reports the top level, so reordering compares it against the
// siblings it actually sits with.
func TestFavoriteNavigationNodesOrphanRendersAtTopLevel(t *testing.T) {
	favorites := []linearapi.Favorite{
		{Type: "project", ID: "fav-orphan", ProjectID: "project-1", ProjectName: "Orphan", ParentID: "gone"},
	}

	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) != 1 {
		t.Fatalf("favoriteNavigationNodes() returned %d roots, want 1", len(nodes))
	}
	if nodes[0].FavoriteParentID != "" {
		t.Errorf("orphan FavoriteParentID = %q, want empty", nodes[0].FavoriteParentID)
	}
}

// TestFavoriteNavigationNodesViews verifies custom view and predefined view
// favorites map onto navigation nodes.
func TestFavoriteNavigationNodesViews(t *testing.T) {
	favorites := []linearapi.Favorite{
		{Type: "customView", CustomViewID: "view-1", CustomViewName: "Open Bugs", Title: "Open Bugs"},
		{Type: "predefinedView", ID: "fav-triage", Title: "Triage", PredefinedViewType: "triage", PredefinedViewTeamID: "team-1"},
		{Type: "predefinedView", ID: "fav-all", Title: "All issues", PredefinedViewType: "allIssues"},
		{Type: "predefinedView", ID: "fav-all-team", Title: "All issues", PredefinedViewType: "allIssues", PredefinedViewTeamID: "team-1"},
		{Type: "predefinedView", ID: "fav-other", Title: "Cycles", PredefinedViewType: "cycles"},
	}

	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) != 4 {
		t.Fatalf("favoriteNavigationNodes() returned %d nodes, want 4: %+v", len(nodes), nodes)
	}
	view := nodes[0]
	if view.CustomViewID != "view-1" || view.Text != "Open Bugs" {
		t.Errorf("custom view node = %+v", view)
	}
	triage := nodes[1]
	if triage.StateType != "triage" || triage.TeamID != "team-1" || triage.Text != "Triage" {
		t.Errorf("triage node = %+v", triage)
	}
	all := nodes[2]
	if all.ID != "all" || all.Text != "All issues" || all.TeamID != "" {
		t.Errorf("workspace all issues node = %+v", all)
	}
	teamAll := nodes[3]
	if teamAll.ID != "all" || teamAll.Text != "All issues" || teamAll.TeamID != "team-1" {
		t.Errorf("team-scoped all issues node = %+v", teamAll)
	}
}

// TestRebuildNavigationTreeOmitsEmptyFavorites verifies the Favorites group
// only renders when displayable favorites exist.
func TestRebuildNavigationTreeOmitsEmptyFavorites(t *testing.T) {
	app := newDefaultNavTestApp(config.Config{})
	teams := []linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}

	app.rebuildNavigationTree(teams, nil)
	if got := len(app.navigationTree.GetRoot().GetChildren()); got != 2 {
		t.Fatalf("root children without favorites = %d, want 2 (All Issues + team)", got)
	}

	app.rebuildNavigationTree(teams, []linearapi.Favorite{{Type: "customView", ID: "fav-1"}})
	if got := len(app.navigationTree.GetRoot().GetChildren()); got != 2 {
		t.Fatalf("root children with only unsupported favorites = %d, want 2", got)
	}

	favorites := []linearapi.Favorite{{Type: "project", ProjectID: "project-1", ProjectName: "Website", ProjectTeamID: "team-1"}}
	app.rebuildNavigationTree(teams, favorites)
	children := app.navigationTree.GetRoot().GetChildren()
	if len(children) != 3 {
		t.Fatalf("root children with favorites = %d, want 3 (All Issues + Favorites + team)", len(children))
	}
	group := children[1]
	if group.GetText() != "Favorites" {
		t.Fatalf("second root child = %q, want Favorites group", group.GetText())
	}
	if got := len(group.GetChildren()); got != 1 {
		t.Fatalf("favorites group children = %d, want 1", got)
	}
}
