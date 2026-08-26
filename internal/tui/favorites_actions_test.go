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
		{ID: "fav-project", Type: "project", ProjectID: "project-1"},
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
			ProjectName: "Website", SortOrder: 1,
		}, nil
	}

	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)

	root := app.navigationTree.GetRoot()
	teamNode := app.findTeamTreeNode("team-1")
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
	if got := root.GetChildren()[2]; got != app.favoritesGroup {
		t.Errorf("favorites group is at index %d, want under All Issues and its blank row", indexOfChild(root, app.favoritesGroup))
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

	teamNode := app.findTeamTreeNode("team-1")
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

	app.navigationTree.SetCurrentNode(app.findTeamTreeNode("team-1"))

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
	app.navigationTree.SetCurrentNode(app.findTeamTreeNode("team-1"))

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

// allExpanded is the folder state a plan test uses when collapse is not what it
// is pinning.
func allExpanded(string) bool { return true }

func TestPlanFavoriteEnterFolderLandsOnTheEndItArrivesAt(t *testing.T) {
	// down: loose steps onto the folder below it and lands above child-a.
	// up: loose steps onto the folder above it and lands below child-b.
	favorites := []linearapi.Favorite{
		{ID: "above", Type: "folder", FolderName: "Above", SortOrder: 10},
		{ID: "child-b", Type: "project", ProjectID: "p1", ParentID: "above", SortOrder: 11},
		{ID: "loose", Type: "project", ProjectID: "p2", SortOrder: 20},
		{ID: "below", Type: "folder", FolderName: "Below", SortOrder: 30},
		{ID: "child-a", Type: "project", ProjectID: "p3", ParentID: "below", SortOrder: 31},
	}

	down, ok := planFavoriteEnterFolder(favorites, "loose", "", 1, allExpanded)
	if !ok || down.ParentID != "below" {
		t.Fatalf("stepping down = %+v, %v; want the folder below", down, ok)
	}
	if down.SortOrder >= 31 {
		t.Errorf("down.SortOrder = %v, want less than the first child's 31", down.SortOrder)
	}

	up, ok := planFavoriteEnterFolder(favorites, "loose", "", -1, allExpanded)
	if !ok || up.ParentID != "above" {
		t.Fatalf("stepping up = %+v, %v; want the folder above", up, ok)
	}
	if up.SortOrder <= 11 {
		t.Errorf("up.SortOrder = %v, want greater than the last child's 11", up.SortOrder)
	}
}

func TestPlanFavoriteEnterFolderStepsOverACollapsedOne(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "loose", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 20},
		{ID: "child", Type: "project", ProjectID: "p2", ParentID: "folder", SortOrder: 21},
	}
	collapsed := func(string) bool { return false }

	if _, ok := planFavoriteEnterFolder(favorites, "loose", "", 1, collapsed); ok {
		t.Error("entered a collapsed folder, want the plain reorder past it")
	}
}

func TestPlanFavoriteEnterFolderRefusesAFolderAndTheEdges(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "first", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 20},
		{ID: "folder-2", Type: "folder", FolderName: "Home", SortOrder: 30},
	}

	// Linear's sidebar has no nested folders.
	if _, ok := planFavoriteEnterFolder(favorites, "folder-2", "", -1, allExpanded); ok {
		t.Error("a folder entered another folder, want false")
	}
	if _, ok := planFavoriteEnterFolder(favorites, "first", "", -1, allExpanded); ok {
		t.Error("entered something above the first row, want false")
	}
}

func TestPlanFavoriteEnterEmptyFolder(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "loose", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 20},
	}

	plan, ok := planFavoriteEnterFolder(favorites, "loose", "", 1, allExpanded)
	if !ok || plan.ParentID != "folder" {
		t.Fatalf("planFavoriteEnterFolder(empty folder) = %+v, %v; want the folder", plan, ok)
	}
}

func TestPlanFavoriteLeaveFolderStepsPastIt(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "before", Type: "project", ProjectID: "p1", SortOrder: 10},
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 20},
		{ID: "only", Type: "project", ProjectID: "p2", ParentID: "folder", SortOrder: 21},
		{ID: "after", Type: "project", ProjectID: "p3", SortOrder: 30},
	}

	down, ok := planFavoriteLeaveFolder(favorites, "only", "folder", 1)
	if !ok || down.ParentID != "" {
		t.Fatalf("stepping down out = %+v, %v; want the top level", down, ok)
	}
	if down.SortOrder <= 20 || down.SortOrder >= 30 {
		t.Errorf("down.SortOrder = %v, want between the folder's 20 and after's 30", down.SortOrder)
	}

	up, ok := planFavoriteLeaveFolder(favorites, "only", "folder", -1)
	if !ok || up.ParentID != "" {
		t.Fatalf("stepping up out = %+v, %v; want the top level", up, ok)
	}
	if up.SortOrder <= 10 || up.SortOrder >= 20 {
		t.Errorf("up.SortOrder = %v, want between before's 10 and the folder's 20", up.SortOrder)
	}
}

func TestPlanFavoriteLeaveFolderClearsATiedNeighbor(t *testing.T) {
	// A half-failed reorder leaves a pair sharing a sort order. A midpoint of
	// two equal values is that value, which lands the favorite tied with both.
	favorites := []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 20},
		{ID: "only", Type: "project", ProjectID: "p1", ParentID: "folder", SortOrder: 21},
		{ID: "tied", Type: "project", ProjectID: "p2", SortOrder: 20},
	}

	down, ok := planFavoriteLeaveFolder(favorites, "only", "folder", 1)
	if !ok {
		t.Fatalf("stepping down out of a folder = %v, want a plan", ok)
	}
	if down.SortOrder <= 20 {
		t.Errorf("down.SortOrder = %v, want past the tied pair at 20", down.SortOrder)
	}

	up, ok := planFavoriteLeaveFolder(favorites, "only", "folder", -1)
	if !ok {
		t.Fatalf("stepping up out of a folder = %v, want a plan", ok)
	}
	if up.SortOrder >= 20 {
		t.Errorf("up.SortOrder = %v, want before the tied pair at 20", up.SortOrder)
	}
}

func TestPlanFavoriteLeaveFolderOnlyAtTheEdges(t *testing.T) {
	favorites := []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "first", Type: "project", ProjectID: "p1", ParentID: "folder", SortOrder: 11},
		{ID: "last", Type: "project", ProjectID: "p2", ParentID: "folder", SortOrder: 12},
		{ID: "loose", Type: "project", ProjectID: "p3", SortOrder: 20},
	}

	if _, ok := planFavoriteLeaveFolder(favorites, "first", "folder", 1); ok {
		t.Error("left the folder with a sibling below, want the plain reorder")
	}
	if _, ok := planFavoriteLeaveFolder(favorites, "last", "folder", -1); ok {
		t.Error("left the folder with a sibling above, want the plain reorder")
	}
	if _, ok := planFavoriteLeaveFolder(favorites, "loose", "", 1); ok {
		t.Error("left the top level, want false")
	}
}

func TestMoveFavoriteWalksIntoAndOutOfAFolder(t *testing.T) {
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
	if !app.moveFavorite(app.currentNavigationNode(), -1) {
		t.Fatal("stepping up into the folder did not land")
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
	if !app.moveFavorite(app.currentNavigationNode(), 1) {
		t.Fatal("stepping down out of the folder did not land")
	}
	waitForFavorites(t, settled)

	if gotParent != "" {
		t.Fatalf("leaving wrote parent %q, want the top level", gotParent)
	}
	if len(app.favoritesGroup.GetChildren()) != 2 {
		t.Fatalf("favorites group has %d top-level children after leaving, want 2",
			len(app.favoritesGroup.GetChildren()))
	}
}

func TestMoveFavoriteStepsOverACollapsedFolder(t *testing.T) {
	app := newFavoritesTestApp(t)
	settled := make(chan struct{}, 1)
	app.favoritesChanged = func() { settled <- struct{}{} }
	app.moveFavoriteFunc = func(context.Context, string, string, float64) error {
		t.Error("moveFavoriteFunc called for a collapsed folder, want a plain reorder")
		return nil
	}
	sorted := map[string]float64{}
	app.updateFavoriteSortFunc = func(_ context.Context, favoriteID string, sortOrder float64) error {
		sorted[favoriteID] = sortOrder
		return nil
	}

	app.rebuildNavigationTree(nil, []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "child", Type: "project", ProjectID: "p1", ProjectName: "Beta", ParentID: "folder", SortOrder: 11},
		{ID: "loose", Type: "project", ProjectID: "p2", ProjectName: "Alpha", SortOrder: 20},
	})
	app.favoritesGroup.GetChildren()[0].SetExpanded(false)

	app.navigationTree.SetCurrentNode(app.favoritesGroup.GetChildren()[1])
	if !app.moveFavorite(app.currentNavigationNode(), -1) {
		t.Fatal("stepping up past a collapsed folder did not land")
	}
	waitForFavorites(t, settled)

	if sorted["loose"] != 10 || sorted["folder"] != 20 {
		t.Fatalf("sort orders = %v, want loose and folder swapped", sorted)
	}
}

func TestCollapsedFavoriteFolderSurvivesARefresh(t *testing.T) {
	app := newFavoritesTestApp(t)
	app.rebuildNavigationTree(nil, []linearapi.Favorite{
		{ID: "folder", Type: "folder", FolderName: "Work", SortOrder: 10},
		{ID: "child", Type: "project", ProjectID: "p1", ProjectName: "Beta", ParentID: "folder", SortOrder: 11},
	})

	app.favoritesGroup.GetChildren()[0].SetExpanded(false)
	app.refreshFavoritesSection("")

	if app.favoritesGroup.GetChildren()[0].IsExpanded() {
		t.Error("the folder reopened across a refresh, want it left collapsed")
	}
}

func TestMoveFavoriteIgnoresNonFavorites(t *testing.T) {
	app := newFavoritesTestApp(t)
	app.moveFavoriteFunc = func(context.Context, string, string, float64) error {
		t.Error("moveFavoriteFunc called for a non-favorite")
		return nil
	}
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.navigationTree.SetCurrentNode(app.findTeamTreeNode("team-1"))

	if app.moveFavorite(app.currentNavigationNode(), 1) {
		t.Error("moveFavorite landed on a team node")
	}
	if app.moveFavorite(app.currentNavigationNode(), -1) {
		t.Error("moveFavorite landed on a team node")
	}
}
