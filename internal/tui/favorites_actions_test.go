package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

// newFavoritesTestApp returns an app whose favorites mutations are stubbed and
// whose UI updates run inline.
func newFavoritesTestApp(t *testing.T) *App {
	t.Helper()
	app := newUXTestApp(t)
	app.createFavoriteFunc = func(context.Context, linearapi.FavoriteTarget) (linearapi.Favorite, error) {
		t.Fatal("createFavoriteFunc called unexpectedly")
		return linearapi.Favorite{}, nil
	}
	app.deleteFavoriteFunc = func(context.Context, string) error {
		t.Fatal("deleteFavoriteFunc called unexpectedly")
		return nil
	}
	app.updateFavoriteSortFunc = func(context.Context, string, float64) error {
		t.Fatal("updateFavoriteSortFunc called unexpectedly")
		return nil
	}
	return app
}

// waitForFavorites blocks until a favorites mutation settles.
func waitForFavorites(t *testing.T, settled <-chan struct{}) {
	t.Helper()
	select {
	case <-settled:
	case <-time.After(2 * time.Second):
		t.Fatal("favorites mutation did not settle")
	}
}

func TestFavoriteTargetForNode(t *testing.T) {
	tests := []struct {
		name string
		node *NavigationNode
		want linearapi.FavoriteTarget
		ok   bool
	}{
		{
			name: "project",
			node: &NavigationNode{ID: "project-1", IsProject: true, TeamID: "team-1"},
			want: linearapi.FavoriteTarget{ProjectID: "project-1"},
			ok:   true,
		},
		{
			name: "team",
			node: &NavigationNode{ID: "team-1", IsTeam: true, TeamID: "team-1"},
			want: linearapi.FavoriteTarget{TeamID: "team-1"},
			ok:   true,
		},
		{
			name: "cycle",
			node: &NavigationNode{ID: "cycle-1", IsCycle: true, CycleID: "cycle-1", TeamID: "team-1"},
			want: linearapi.FavoriteTarget{CycleID: "cycle-1"},
			ok:   true,
		},
		{
			name: "custom view wins over team",
			node: &NavigationNode{ID: "view-1", CustomViewID: "view-1", TeamID: "team-1", IsTeam: true},
			want: linearapi.FavoriteTarget{CustomViewID: "view-1"},
			ok:   true,
		},
		{
			name: "triage",
			node: &NavigationNode{ID: "fav-1", StateType: "triage", TeamID: "team-1"},
			want: linearapi.FavoriteTarget{PredefinedViewType: "triage", PredefinedViewTeamID: "team-1"},
			ok:   true,
		},
		{
			name: "issue",
			node: &NavigationNode{ID: "issue-1", IsIssue: true, IssueID: "issue-1"},
			want: linearapi.FavoriteTarget{IssueID: "issue-1"},
			ok:   true,
		},
		{
			name: "all issues",
			node: &NavigationNode{ID: "all", Text: "All Issues"},
		},
		{
			name: "workflow status",
			node: &NavigationNode{ID: "state-1", IsStatus: true, StateID: "state-1", TeamID: "team-1"},
		},
		{
			name: "cycles group header",
			node: &NavigationNode{ID: "team-1-cycles", Text: "Cycles", IsCycle: true, TeamID: "team-1"},
		},
		{
			name: "favorites folder",
			node: &NavigationNode{ID: "folder-1", IsFolder: true, FavoriteID: "folder-1"},
		},
		{
			name: "nil node",
			node: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := favoriteTargetForNode(tt.node)
			if ok != tt.ok {
				t.Fatalf("favoriteTargetForNode() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("favoriteTargetForNode() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFavoriteForNodeMatchesEntityWithoutFavoriteID(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "fav-team", Type: "team", TeamID: "team-1"},
		{ID: "fav-project", Type: "project", ProjectID: "project-1", ProjectTeamID: "team-1"},
		{ID: "fav-triage", Type: "predefinedView", PredefinedViewType: "triage", PredefinedViewTeamID: "team-2"},
	}

	// A project node under a team carries no FavoriteID, so the match has to
	// come from the entity id.
	project := &NavigationNode{ID: "project-1", IsProject: true, TeamID: "team-1"}
	got, ok := favoriteForNode(favorites, project)
	if !ok || got.ID != "fav-project" {
		t.Fatalf("favoriteForNode(project) = %+v, %v; want fav-project", got, ok)
	}

	team := &NavigationNode{ID: "team-1", IsTeam: true, TeamID: "team-1"}
	got, ok = favoriteForNode(favorites, team)
	if !ok || got.ID != "fav-team" {
		t.Fatalf("favoriteForNode(team) = %+v, %v; want fav-team", got, ok)
	}

	// The project favorite records team-1 as its team, but that must not make
	// it match a team node.
	otherTeam := &NavigationNode{ID: "team-9", IsTeam: true, TeamID: "team-9"}
	if got, ok = favoriteForNode(favorites, otherTeam); ok {
		t.Fatalf("favoriteForNode(unfavorited team) = %+v, want no match", got)
	}

	triage := &NavigationNode{ID: "x", StateType: "triage", TeamID: "team-2"}
	if got, ok = favoriteForNode(favorites, triage); !ok || got.ID != "fav-triage" {
		t.Fatalf("favoriteForNode(triage) = %+v, %v; want fav-triage", got, ok)
	}

	// Same predefined view, different team, is a different favorite.
	if got, ok = favoriteForNode(favorites, &NavigationNode{StateType: "triage", TeamID: "team-3"}); ok {
		t.Fatalf("favoriteForNode(other team triage) = %+v, want no match", got)
	}
}

func TestFavoriteSiblingsSkipsUnrenderableAndOtherFolders(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "folder-1", Type: "folder", FolderName: "Work", SortOrder: 1},
		{ID: "root-a", Type: "project", ProjectID: "p1", SortOrder: 2},
		{ID: "nested", Type: "project", ProjectID: "p2", ParentID: "folder-1", SortOrder: 3},
		{ID: "doc", Type: "document", SortOrder: 4},
		{ID: "root-b", Type: "team", TeamID: "t1", SortOrder: 5},
		{ID: "orphan", Type: "team", TeamID: "t2", ParentID: "missing-folder", SortOrder: 6},
	}

	roots := favoriteSiblings(favorites, "")
	var rootIDs []string
	for _, favorite := range roots {
		rootIDs = append(rootIDs, favorite.ID)
	}
	// The document is unrenderable and drops out. The orphan's folder is
	// missing, so it renders at the top level and counts as a sibling there.
	want := []string{"folder-1", "root-a", "root-b", "orphan"}
	if len(rootIDs) != len(want) {
		t.Fatalf("favoriteSiblings(root) = %v, want %v", rootIDs, want)
	}
	for i := range want {
		if rootIDs[i] != want[i] {
			t.Fatalf("favoriteSiblings(root) = %v, want %v", rootIDs, want)
		}
	}

	nested := favoriteSiblings(favorites, "folder-1")
	if len(nested) != 1 || nested[0].ID != "nested" {
		t.Fatalf("favoriteSiblings(folder-1) = %+v, want just nested", nested)
	}
}

func TestPlanFavoriteReorderSwapsSortOrders(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "a", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "b", Type: "project", ProjectID: "p2", SortOrder: 20},
		{ID: "c", Type: "project", ProjectID: "p3", SortOrder: 30},
	}

	plan, ok := planFavoriteReorder(favorites, "b", "", -1)
	if !ok {
		t.Fatal("planFavoriteReorder(b, up) ok = false, want true")
	}
	want := favoriteReorder{MovedID: "b", MovedSort: 10, NeighborID: "a", NeighborSort: 20}
	if plan != want {
		t.Fatalf("planFavoriteReorder(b, up) = %+v, want %+v", plan, want)
	}

	plan, ok = planFavoriteReorder(favorites, "b", "", 1)
	if !ok {
		t.Fatal("planFavoriteReorder(b, down) ok = false, want true")
	}
	want = favoriteReorder{MovedID: "b", MovedSort: 30, NeighborID: "c", NeighborSort: 20}
	if plan != want {
		t.Fatalf("planFavoriteReorder(b, down) = %+v, want %+v", plan, want)
	}
}

func TestPlanFavoriteReorderNoOpAtEnds(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "a", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "b", Type: "project", ProjectID: "p2", SortOrder: 20},
	}

	if _, ok := planFavoriteReorder(favorites, "a", "", -1); ok {
		t.Error("planFavoriteReorder(first, up) ok = true, want false")
	}
	if _, ok := planFavoriteReorder(favorites, "b", "", 1); ok {
		t.Error("planFavoriteReorder(last, down) ok = true, want false")
	}
	if _, ok := planFavoriteReorder(favorites, "missing", "", 1); ok {
		t.Error("planFavoriteReorder(unknown) ok = true, want false")
	}
}

func TestPlanFavoriteReorderStaysWithinFolder(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "root-a", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 20},
		{ID: "child-a", Type: "project", ProjectID: "p2", ParentID: "folder", SortOrder: 30},
		{ID: "child-b", Type: "project", ProjectID: "p3", ParentID: "folder", SortOrder: 40},
	}

	// The first child of a folder cannot move up out of it.
	if _, ok := planFavoriteReorder(favorites, "child-a", "folder", -1); ok {
		t.Error("planFavoriteReorder(first child, up) ok = true, want false")
	}

	plan, ok := planFavoriteReorder(favorites, "child-a", "folder", 1)
	if !ok {
		t.Fatal("planFavoriteReorder(child-a, down) ok = false, want true")
	}
	if plan.NeighborID != "child-b" {
		t.Fatalf("planFavoriteReorder(child-a, down) neighbor = %q, want child-b", plan.NeighborID)
	}
}

func TestRemoveFavoriteByIDDropsFolderChildren(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "folder", Type: "folder"},
		{ID: "child", Type: "project", ParentID: "folder"},
		{ID: "other", Type: "project"},
	}

	got := removeFavoriteByID(favorites, "folder")
	if len(got) != 1 || got[0].ID != "other" {
		t.Fatalf("removeFavoriteByID(folder) = %+v, want just other", got)
	}
}

func TestUpsertFavoriteReplacesAndSorts(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "a", SortOrder: 10},
		{ID: "b", SortOrder: 30},
	}

	got := upsertFavorite(favorites, linearapi.Favorite{ID: "c", SortOrder: 20})
	if len(got) != 3 || got[1].ID != "c" {
		t.Fatalf("upsertFavorite(new) = %+v, want c in the middle", got)
	}

	got = upsertFavorite(got, linearapi.Favorite{ID: "c", SortOrder: 40})
	if len(got) != 3 || got[2].ID != "c" {
		t.Fatalf("upsertFavorite(existing) = %+v, want c last and no duplicate", got)
	}
}

func TestToggleFavoriteAddsProjectAndKeepsTeamExpanded(t *testing.T) {
	app := newFavoritesTestApp(t)
	settled := make(chan struct{}, 1)
	app.favoritesChanged = func() { settled <- struct{}{} }

	var gotTarget linearapi.FavoriteTarget
	app.createFavoriteFunc = func(_ context.Context, target linearapi.FavoriteTarget) (linearapi.Favorite, error) {
		gotTarget = target
		return linearapi.Favorite{
			ID: "fav-1", Type: "project", ProjectID: "project-1",
			ProjectName: "Website", ProjectTeamID: "team-1", SortOrder: 1,
		}, nil
	}

	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)

	root := app.navigationTree.GetRoot()
	teamNode := root.GetChildren()[1]
	app.populateTeamNodeChildren(teamNode, "team-1", []linearapi.Project{{ID: "project-1", Name: "Website", TeamID: "team-1"}}, nil, nil)
	teamNode.SetExpanded(true)
	projectNode := teamNode.GetChildren()[0]
	app.navigationTree.SetCurrentNode(projectNode)

	handleToggleFavorite(app)
	waitForFavorites(t, settled)

	if gotTarget.ProjectID != "project-1" {
		t.Fatalf("create target = %+v, want ProjectID project-1", gotTarget)
	}
	if len(app.favorites) != 1 || app.favorites[0].ID != "fav-1" {
		t.Fatalf("app.favorites = %+v, want the created favorite", app.favorites)
	}
	if app.favoritesGroup == nil {
		t.Fatal("favorites group was not rendered")
	}
	if got := root.GetChildren()[1]; got != app.favoritesGroup {
		t.Errorf("favorites group is at index %d, want directly under All Issues", indexOfChild(root, app.favoritesGroup))
	}
	if !teamNode.IsExpanded() {
		t.Error("team collapsed after favoriting; the section refresh must not rebuild the whole tree")
	}
	if app.navigationTree.GetCurrentNode() != projectNode {
		t.Error("cursor moved away from the project that was favorited")
	}
}

func TestToggleFavoriteRemovesExistingFavorite(t *testing.T) {
	app := newFavoritesTestApp(t)
	settled := make(chan struct{}, 1)
	app.favoritesChanged = func() { settled <- struct{}{} }

	var deletedID string
	app.deleteFavoriteFunc = func(_ context.Context, favoriteID string) error {
		deletedID = favoriteID
		return nil
	}

	favorites := []linearapi.Favorite{
		{ID: "fav-1", Type: "project", ProjectID: "project-1", ProjectName: "Website", SortOrder: 1},
	}
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, favorites)

	group := app.favoritesGroup
	if group == nil {
		t.Fatal("favorites group missing")
	}
	app.navigationTree.SetCurrentNode(group.GetChildren()[0])

	handleToggleFavorite(app)
	waitForFavorites(t, settled)

	if deletedID != "fav-1" {
		t.Fatalf("deleted favorite id = %q, want fav-1", deletedID)
	}
	if len(app.favorites) != 0 {
		t.Fatalf("app.favorites = %+v, want empty", app.favorites)
	}
	if app.favoritesGroup != nil {
		t.Error("favorites group still rendered with no favorites left")
	}
	root := app.navigationTree.GetRoot()
	if indexOfChild(root, group) >= 0 {
		t.Error("stale favorites group still attached to the root")
	}
	current := app.navigationTree.GetCurrentNode()
	if current == nil || !treeContains(root, current) {
		t.Error("cursor left dangling on a detached node")
	}
}

func TestToggleFavoriteRejectsWorkflowStatus(t *testing.T) {
	app := newFavoritesTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)

	root := app.navigationTree.GetRoot()
	teamNode := root.GetChildren()[1]
	app.populateTeamNodeChildren(teamNode, "team-1", nil, []linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}}, nil)
	statusNode := teamNode.GetChildren()[0].GetChildren()[0]
	app.navigationTree.SetCurrentNode(statusNode)

	// The stubs fail the test if any mutation fires. Linear has no favorite
	// type for a workflow state, so none should.
	handleToggleFavorite(app)

	if len(app.favorites) != 0 {
		t.Fatalf("app.favorites = %+v, want empty", app.favorites)
	}
}

func TestMoveFavoriteWritesSwappedSortOrders(t *testing.T) {
	app := newFavoritesTestApp(t)
	settled := make(chan struct{}, 1)
	app.favoritesChanged = func() { settled <- struct{}{} }

	writes := map[string]float64{}
	app.updateFavoriteSortFunc = func(_ context.Context, favoriteID string, sortOrder float64) error {
		writes[favoriteID] = sortOrder
		return nil
	}

	favorites := []linearapi.Favorite{
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 10},
		{ID: "fav-b", Type: "project", ProjectID: "p2", ProjectName: "Beta", SortOrder: 20},
	}
	app.rebuildNavigationTree(nil, favorites)

	betaNode := app.favoritesGroup.GetChildren()[1]
	app.navigationTree.SetCurrentNode(betaNode)

	app.moveFavorite(app.currentNavigationNode(), -1)
	waitForFavorites(t, settled)

	if writes["fav-b"] != 10 || writes["fav-a"] != 20 {
		t.Fatalf("sort order writes = %v, want fav-b 10 and fav-a 20", writes)
	}
	if app.favorites[0].ID != "fav-b" || app.favorites[1].ID != "fav-a" {
		t.Fatalf("app.favorites order = %q, %q; want fav-b first", app.favorites[0].ID, app.favorites[1].ID)
	}
	current := app.navigationTree.GetCurrentNode()
	ref, _ := current.GetReference().(*NavigationNode)
	if ref == nil || ref.FavoriteID != "fav-b" {
		t.Error("cursor did not follow the favorite that moved")
	}
}

func TestMoveFavoriteAtTopDoesNotCallTheAPI(t *testing.T) {
	app := newFavoritesTestApp(t)
	favorites := []linearapi.Favorite{
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 10},
		{ID: "fav-b", Type: "project", ProjectID: "p2", ProjectName: "Beta", SortOrder: 20},
	}
	app.rebuildNavigationTree(nil, favorites)

	app.navigationTree.SetCurrentNode(app.favoritesGroup.GetChildren()[0])

	// The stub fails the test if it is called.
	if app.moveFavorite(app.currentNavigationNode(), -1) {
		t.Error("moveFavorite consumed the key at the top of the list")
	}
}

func TestMoveFavoriteRejectsNonFavoriteNode(t *testing.T) {
	app := newFavoritesTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)

	root := app.navigationTree.GetRoot()
	app.navigationTree.SetCurrentNode(root.GetChildren()[1])

	// The stub fails the test if it is called.
	if app.moveFavorite(app.currentNavigationNode(), 1) {
		t.Error("moveFavorite consumed the key on a team node")
	}
}

func TestFavoritesMutationErrorLeavesStateAlone(t *testing.T) {
	app := newFavoritesTestApp(t)
	settled := make(chan struct{}, 1)
	app.favoritesChanged = func() { settled <- struct{}{} }
	app.createFavoriteFunc = func(context.Context, linearapi.FavoriteTarget) (linearapi.Favorite, error) {
		return linearapi.Favorite{}, errors.New("boom")
	}

	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.navigationTree.SetCurrentNode(app.navigationTree.GetRoot().GetChildren()[1])

	handleToggleFavorite(app)
	waitForFavorites(t, settled)

	if len(app.favorites) != 0 {
		t.Fatalf("app.favorites = %+v, want empty after a failed create", app.favorites)
	}
	if app.favoritesGroup != nil {
		t.Error("favorites group rendered after a failed create")
	}
}

// indexOfChild returns the position of a child under a tree node, or -1.
func indexOfChild(parent, target *tview.TreeNode) int {
	for i, child := range parent.GetChildren() {
		if child == target {
			return i
		}
	}
	return -1
}

func TestPlanFavoriteNestIntoFolderAbove(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "child", Type: "project", ProjectID: "p1", ParentID: "folder", SortOrder: 20},
		{ID: "loose", Type: "project", ProjectID: "p2", SortOrder: 30},
	}

	plan, ok := planFavoriteNest(favorites, "loose", "")
	if !ok {
		t.Fatal("planFavoriteNest(loose) ok = false, want true")
	}
	if plan.ParentID != "folder" {
		t.Errorf("plan.ParentID = %q, want folder", plan.ParentID)
	}
	// It lands below the folder's existing child.
	if plan.SortOrder <= 20 {
		t.Errorf("plan.SortOrder = %v, want greater than the last child's 20", plan.SortOrder)
	}
}

func TestPlanFavoriteNestIntoEmptyFolder(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "loose", Type: "project", ProjectID: "p1", SortOrder: 20},
	}

	plan, ok := planFavoriteNest(favorites, "loose", "")
	if !ok || plan.ParentID != "folder" {
		t.Fatalf("planFavoriteNest(empty folder) = %+v, %v; want parent folder", plan, ok)
	}
}

func TestPlanFavoriteNestRejectsNonFolderAboveAndFolders(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "first", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "second", Type: "project", ProjectID: "p2", SortOrder: 20},
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 30},
		{ID: "folder-2", Type: "folder", FolderName: "Home", SortOrder: 40},
	}

	if _, ok := planFavoriteNest(favorites, "second", ""); ok {
		t.Error("nested under a project, want false")
	}
	if _, ok := planFavoriteNest(favorites, "first", ""); ok {
		t.Error("nested the first item with nothing above it, want false")
	}
	// Linear's sidebar has no nested folders.
	if _, ok := planFavoriteNest(favorites, "folder-2", ""); ok {
		t.Error("nested a folder inside a folder, want false")
	}
}

func TestPlanFavoriteUnnestLandsBelowItsFolder(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "child", Type: "project", ProjectID: "p1", ParentID: "folder", SortOrder: 15},
		{ID: "after", Type: "project", ProjectID: "p2", SortOrder: 20},
	}

	plan, ok := planFavoriteUnnest(favorites, "child", "folder")
	if !ok {
		t.Fatal("planFavoriteUnnest(child) ok = false, want true")
	}
	if plan.ParentID != "" {
		t.Errorf("plan.ParentID = %q, want the top level", plan.ParentID)
	}
	if plan.SortOrder <= 10 || plan.SortOrder >= 20 {
		t.Errorf("plan.SortOrder = %v, want between the folder's 10 and the next item's 20", plan.SortOrder)
	}
}

func TestPlanFavoriteUnnestAtTopLevelIsNoOp(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "loose", Type: "project", ProjectID: "p1", SortOrder: 10},
	}

	if _, ok := planFavoriteUnnest(favorites, "loose", ""); ok {
		t.Error("planFavoriteUnnest(top level) ok = true, want false")
	}
}

func TestNestFavoriteRoundTripsThroughTheTree(t *testing.T) {
	app := newFavoritesTestApp(t)
	settled := make(chan struct{}, 1)
	app.favoritesChanged = func() { settled <- struct{}{} }

	var gotID, gotParent string
	app.moveFavoriteFunc = func(_ context.Context, favoriteID, parentID string, _ float64) error {
		gotID, gotParent = favoriteID, parentID
		return nil
	}

	app.rebuildNavigationTree(nil, []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "loose", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 20},
	})

	// Second child of the group is the loose project; the folder is first.
	app.navigationTree.SetCurrentNode(app.favoritesGroup.GetChildren()[1])
	if !app.nestFavorite(app.currentNavigationNode(), false) {
		t.Fatal("nestFavorite did not consume the key")
	}
	waitForFavorites(t, settled)

	if gotID != "loose" || gotParent != "folder" {
		t.Fatalf("move wrote %q into %q, want loose into folder", gotID, gotParent)
	}
	if len(app.favoritesGroup.GetChildren()) != 1 {
		t.Fatalf("favorites group has %d top-level children, want just the folder",
			len(app.favoritesGroup.GetChildren()))
	}
	folderNode := app.favoritesGroup.GetChildren()[0]
	if len(folderNode.GetChildren()) != 1 {
		t.Fatalf("folder has %d children, want the nested project", len(folderNode.GetChildren()))
	}

	// And back out again.
	app.navigationTree.SetCurrentNode(folderNode.GetChildren()[0])
	if !app.nestFavorite(app.currentNavigationNode(), true) {
		t.Fatal("unnest did not consume the key")
	}
	waitForFavorites(t, settled)

	if gotParent != "" {
		t.Fatalf("unnest wrote parent %q, want the top level", gotParent)
	}
	if len(app.favoritesGroup.GetChildren()) != 2 {
		t.Fatalf("favorites group has %d top-level children after unnesting, want 2",
			len(app.favoritesGroup.GetChildren()))
	}
}

func TestNestFavoriteIgnoresNonFavorites(t *testing.T) {
	app := newFavoritesTestApp(t)
	app.moveFavoriteFunc = func(context.Context, string, string, float64) error {
		t.Error("moveFavoriteFunc called for a non-favorite")
		return nil
	}
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.navigationTree.SetCurrentNode(app.navigationTree.GetRoot().GetChildren()[1])

	if app.nestFavorite(app.currentNavigationNode(), false) {
		t.Error("nestFavorite consumed the key on a team node")
	}
	if app.nestFavorite(app.currentNavigationNode(), true) {
		t.Error("unnest consumed the key on a team node")
	}
}
