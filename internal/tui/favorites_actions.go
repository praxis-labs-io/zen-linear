package tui

import (
	"context"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

// Linear has no favorite type for a workflow state, so status nodes report false.
func favoriteTargetForNode(node *NavigationNode) (linearapi.FavoriteTarget, bool) {
	if node == nil || node.IsFolder || node.IsGroup {
		return linearapi.FavoriteTarget{}, false
	}
	switch {
	case node.CustomViewID != "":
		return linearapi.FavoriteTarget{CustomViewID: node.CustomViewID}, true
	case node.StateType == "triage":
		return linearapi.FavoriteTarget{
			PredefinedViewType:   "triage",
			PredefinedViewTeamID: node.TeamID,
		}, true
	case node.IsIssue && node.IssueID != "":
		return linearapi.FavoriteTarget{IssueID: node.IssueID}, true
	case node.IsCycle && node.CycleID != "":
		return linearapi.FavoriteTarget{CycleID: node.CycleID}, true
	case node.IsProject && node.ID != "":
		return linearapi.FavoriteTarget{ProjectID: node.ID}, true
	case node.IsTeam && node.TeamID != "":
		return linearapi.FavoriteTarget{TeamID: node.TeamID}, true
	}
	return linearapi.FavoriteTarget{}, false
}

func favoriteMatchesTarget(favorite linearapi.Favorite, target linearapi.FavoriteTarget) bool {
	switch {
	case target.CustomViewID != "":
		return favorite.CustomViewID == target.CustomViewID
	case target.PredefinedViewType != "":
		return favorite.PredefinedViewType == target.PredefinedViewType &&
			favorite.PredefinedViewTeamID == target.PredefinedViewTeamID
	case target.IssueID != "":
		return favorite.IssueID == target.IssueID
	case target.CycleID != "":
		return favorite.CycleID == target.CycleID
	case target.ProjectID != "":
		return favorite.ProjectID == target.ProjectID
	case target.TeamID != "":
		return favorite.TeamID == target.TeamID
	}
	return false
}

func favoriteForNode(favorites []linearapi.Favorite, node *NavigationNode) (linearapi.Favorite, bool) {
	if node == nil {
		return linearapi.Favorite{}, false
	}
	if node.FavoriteID != "" {
		for _, favorite := range favorites {
			if favorite.ID == node.FavoriteID {
				return favorite, true
			}
		}
	}
	target, ok := favoriteTargetForNode(node)
	if !ok {
		return linearapi.Favorite{}, false
	}
	for _, favorite := range favorites {
		if favoriteMatchesTarget(favorite, target) {
			return favorite, true
		}
	}
	return linearapi.Favorite{}, false
}

func (a *App) currentNavigationNode() *NavigationNode {
	if a.navigationTree == nil {
		return nil
	}
	current := a.navigationTree.GetCurrentNode()
	if current == nil {
		return nil
	}
	node, _ := current.GetReference().(*NavigationNode)
	return node
}

func handleToggleFavorite(a *App) {
	node := a.currentNavigationNode()
	if node == nil {
		a.updateStatusBarWithError(fmt.Errorf("no navigation item selected"))
		return
	}
	if existing, ok := favoriteForNode(a.favorites, node); ok {
		a.removeFavorite(existing)
		return
	}
	target, ok := favoriteTargetForNode(node)
	if !ok {
		a.updateStatusBarWithError(fmt.Errorf("%q can't be favorited", node.Text))
		return
	}
	a.addFavorite(node.Text, target)
}

func (a *App) addFavorite(label string, target linearapi.FavoriteTarget) {
	var created linearapi.Favorite
	a.runFavoriteAction(
		func(ctx context.Context) error {
			var err error
			created, err = a.createFavoriteFunc(ctx, target)
			return err
		},
		func() {
			a.favorites = upsertFavorite(a.favorites, created)
			a.refreshFavoritesSection("")
			a.flashSuccess("Favorited " + label)
		},
		"create failed label=%s", label,
	)
}

func (a *App) removeFavorite(favorite linearapi.Favorite) {
	label := favoriteLabel(favorite)
	a.runFavoriteAction(
		func(ctx context.Context) error {
			return a.deleteFavoriteFunc(ctx, favorite.ID)
		},
		func() {
			a.favorites = removeFavoriteByID(a.favorites, favorite.ID)
			a.refreshFavoritesSection("")
			a.flashSuccess("Unfavorited " + label)
		},
		"delete failed favorite_id=%s", favorite.ID,
	)
}

type favoriteReorder struct {
	MovedID      string
	MovedSort    float64
	NeighborID   string
	NeighborSort float64
}

// Swaps the two existing sort orders, so repeated moves cannot drift the way interpolation does.
func planFavoriteReorder(favorites []linearapi.Favorite, favoriteID, parentID string, delta int) (favoriteReorder, bool) {
	siblings := favoriteSiblings(favorites, parentID)
	index := indexOfFavorite(siblings, favoriteID)
	if index < 0 {
		return favoriteReorder{}, false
	}
	swapWith := index + delta
	if swapWith < 0 || swapWith >= len(siblings) {
		return favoriteReorder{}, false
	}

	moved, neighbor := siblings[index], siblings[swapWith]
	return favoriteReorder{
		MovedID:      moved.ID,
		MovedSort:    neighbor.SortOrder,
		NeighborID:   neighbor.ID,
		NeighborSort: moved.SortOrder,
	}, true
}

type favoriteMove struct {
	FavoriteID string
	ParentID   string
	SortOrder  float64
}

func planFavoriteEnterFolder(favorites []linearapi.Favorite, favoriteID, parentID string, delta int, isExpanded func(folderID string) bool) (favoriteMove, bool) {
	siblings := favoriteSiblings(favorites, parentID)
	index := indexOfFavorite(siblings, favoriteID)
	if index < 0 || siblings[index].Type == favoriteTypeFolder {
		return favoriteMove{}, false
	}
	next := index + delta
	if next < 0 || next >= len(siblings) {
		return favoriteMove{}, false
	}
	folder := siblings[next]
	if folder.Type != favoriteTypeFolder || !isExpanded(folder.ID) {
		return favoriteMove{}, false
	}

	children := favoriteSiblings(favorites, folder.ID)
	sortOrder := folder.SortOrder + 1
	switch {
	case len(children) == 0:
	case delta > 0:
		sortOrder = children[0].SortOrder - 1
	default:
		sortOrder = children[len(children)-1].SortOrder + 1
	}
	return favoriteMove{FavoriteID: favoriteID, ParentID: folder.ID, SortOrder: sortOrder}, true
}

func planFavoriteLeaveFolder(favorites []linearapi.Favorite, favoriteID, parentID string, delta int) (favoriteMove, bool) {
	if parentID == "" {
		return favoriteMove{}, false
	}
	siblings := favoriteSiblings(favorites, parentID)
	index := indexOfFavorite(siblings, favoriteID)
	if index < 0 {
		return favoriteMove{}, false
	}
	if next := index + delta; next >= 0 && next < len(siblings) {
		return favoriteMove{}, false
	}
	folder, ok := favoriteByID(favorites, parentID)
	if !ok {
		return favoriteMove{}, false
	}

	grandparent := favoriteParentIDs(favorites)[folder.ID]
	outer := favoriteSiblings(favorites, grandparent)
	folderIndex := indexOfFavorite(outer, folder.ID)

	sortOrder := folder.SortOrder + float64(delta)
	if neighbor := folderIndex + delta; folderIndex >= 0 && neighbor >= 0 && neighbor < len(outer) {
		if gap := outer[neighbor].SortOrder - folder.SortOrder; gap != 0 {
			sortOrder = folder.SortOrder + gap/2
		}
	}
	return favoriteMove{FavoriteID: favoriteID, ParentID: grandparent, SortOrder: sortOrder}, true
}

func (a *App) reparentFavorite(plan favoriteMove) {
	a.runFavoriteAction(
		func(ctx context.Context) error {
			return a.moveFavoriteFunc(ctx, plan.FavoriteID, plan.ParentID, plan.SortOrder)
		},
		func() {
			a.favorites = applyFavoriteMove(a.favorites, plan)
			a.refreshFavoritesSection(plan.FavoriteID)
		},
		"move failed favorite_id=%s", plan.FavoriteID,
	)
}

func (a *App) favoriteFolderExpanded(folderID string) bool {
	node := findFavoriteTreeNode(a.favoritesGroup, folderID)
	return node != nil && node.IsExpanded()
}

func applyFavoriteMove(favorites []linearapi.Favorite, plan favoriteMove) []linearapi.Favorite {
	updated := make([]linearapi.Favorite, len(favorites))
	copy(updated, favorites)
	for i := range updated {
		if updated[i].ID != plan.FavoriteID {
			continue
		}
		updated[i].ParentID = plan.ParentID
		updated[i].SortOrder = plan.SortOrder
	}
	linearapi.SortFavorites(updated)
	return updated
}

func indexOfFavorite(favorites []linearapi.Favorite, favoriteID string) int {
	for i, favorite := range favorites {
		if favorite.ID == favoriteID {
			return i
		}
	}
	return -1
}

func favoriteByID(favorites []linearapi.Favorite, favoriteID string) (linearapi.Favorite, bool) {
	if i := indexOfFavorite(favorites, favoriteID); i >= 0 {
		return favorites[i], true
	}
	return linearapi.Favorite{}, false
}

func (a *App) moveFavorite(node *NavigationNode, delta int) bool {
	if node == nil || node.FavoriteID == "" {
		return false
	}

	if plan, ok := planFavoriteEnterFolder(a.favorites, node.FavoriteID, node.FavoriteParentID, delta, a.favoriteFolderExpanded); ok {
		a.reparentFavorite(plan)
		return true
	}
	if plan, ok := planFavoriteLeaveFolder(a.favorites, node.FavoriteID, node.FavoriteParentID, delta); ok {
		a.reparentFavorite(plan)
		return true
	}

	plan, ok := planFavoriteReorder(a.favorites, node.FavoriteID, node.FavoriteParentID, delta)
	if !ok {
		return false
	}

	go func() {
		ctx := context.Background()
		if err := a.updateFavoriteSortFunc(ctx, plan.MovedID, plan.MovedSort); err != nil {
			a.reportFavoriteError(err, "reorder failed favorite_id=%s", plan.MovedID)
			return
		}
		if err := a.updateFavoriteSortFunc(ctx, plan.NeighborID, plan.NeighborSort); err != nil {
			a.reportFavoriteError(err, "reorder failed favorite_id=%s", plan.NeighborID)
			return
		}
		a.QueueUpdateDraw(func() {
			a.favorites = applyFavoriteSortOrders(a.favorites, map[string]float64{
				plan.MovedID:    plan.MovedSort,
				plan.NeighborID: plan.NeighborSort,
			})
			a.refreshFavoritesSection(plan.MovedID)
			a.favoritesSettled()
		})
	}()
	return true
}

func (a *App) runFavoriteAction(write func(context.Context) error, onSuccess func(), errFmt string, errArgs ...interface{}) {
	go func() {
		if err := write(context.Background()); err != nil {
			a.reportFavoriteError(err, errFmt, errArgs...)
			return
		}
		a.QueueUpdateDraw(func() {
			onSuccess()
			a.favoritesSettled()
		})
	}()
}

func (a *App) reportFavoriteError(err error, format string, args ...interface{}) {
	logger.ErrorWithErr(err, "tui.favorites: "+format, args...)
	a.QueueUpdateDraw(func() {
		a.updateStatusBarWithError(err)
		a.favoritesSettled()
	})
}

// Exists so tests can wait on the favorites goroutine.
func (a *App) favoritesSettled() {
	if a.favoritesChanged != nil {
		a.favoritesChanged()
	}
}

func favoriteSiblings(favorites []linearapi.Favorite, parentID string) []linearapi.Favorite {
	parents := favoriteParentIDs(favorites)
	siblings := make([]linearapi.Favorite, 0, len(favorites))
	for _, favorite := range favorites {
		if parents[favorite.ID] != parentID || !isRenderableFavorite(favorite) {
			continue
		}
		siblings = append(siblings, favorite)
	}
	linearapi.SortFavorites(siblings)
	return siblings
}

func upsertFavorite(favorites []linearapi.Favorite, favorite linearapi.Favorite) []linearapi.Favorite {
	updated := make([]linearapi.Favorite, 0, len(favorites)+1)
	replaced := false
	for _, existing := range favorites {
		if existing.ID == favorite.ID {
			updated = append(updated, favorite)
			replaced = true
			continue
		}
		updated = append(updated, existing)
	}
	if !replaced {
		updated = append(updated, favorite)
	}
	linearapi.SortFavorites(updated)
	return updated
}

func removeFavoriteByID(favorites []linearapi.Favorite, favoriteID string) []linearapi.Favorite {
	updated := make([]linearapi.Favorite, 0, len(favorites))
	for _, favorite := range favorites {
		if favorite.ID == favoriteID || favorite.ParentID == favoriteID {
			continue
		}
		updated = append(updated, favorite)
	}
	return updated
}

func applyFavoriteSortOrders(favorites []linearapi.Favorite, orders map[string]float64) []linearapi.Favorite {
	updated := make([]linearapi.Favorite, len(favorites))
	copy(updated, favorites)
	for i := range updated {
		if sortOrder, ok := orders[updated[i].ID]; ok {
			updated[i].SortOrder = sortOrder
		}
	}
	linearapi.SortFavorites(updated)
	return updated
}

func favoriteLabel(favorite linearapi.Favorite) string {
	if favorite.Title != "" {
		return favorite.Title
	}
	if node := favoriteLeafNode(favorite); node != nil {
		return node.Text
	}
	if favorite.FolderName != "" {
		return favorite.FolderName
	}
	return favorite.Type
}

// Rebuilds only the Favorites group: rebuildNavigationTree would drop team expansion and reset the cursor.
func (a *App) refreshFavoritesSection(preferFavoriteID string) {
	if a.navigationTree == nil {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}

	if a.allIssuesNode == nil {
		return
	}

	previous := a.favoritesGroup
	group := a.buildFavoritesGroup(a.favorites)
	if previous == nil && group == nil {
		return
	}
	restoreFavoriteExpansion(previous, group)

	for _, child := range root.GetChildren() {
		if child != a.allIssuesNode && child != a.teamsGroup {
			a.forgetNavNodeLabels(child)
		}
	}

	a.favoritesGroup = group
	root.SetChildren(a.navRootChildren())
	a.applyNavigationNodeColors(root)
	a.applyNavSelectionStyle(root)
	a.restoreNavigationCursor(root, group, preferFavoriteID)
	a.recordNavCacheAsync()
}

func (a *App) restoreNavigationCursor(root, group *tview.TreeNode, preferFavoriteID string) {
	if preferFavoriteID != "" {
		if node := findFavoriteTreeNode(group, preferFavoriteID); node != nil {
			a.navigationTree.SetCurrentNode(node)
			return
		}
	}
	if current := a.navigationTree.GetCurrentNode(); current != nil && treeContains(root, current) {
		return
	}
	if node := firstSelectableChild(group); node != nil {
		a.navigationTree.SetCurrentNode(node)
		return
	}
	if children := root.GetChildren(); len(children) > 0 {
		a.navigationTree.SetCurrentNode(children[0])
	}
}

func restoreFavoriteExpansion(previous, group *tview.TreeNode) {
	if previous == nil || group == nil {
		return
	}
	for _, child := range group.GetChildren() {
		ref, ok := child.GetReference().(*NavigationNode)
		if !ok || !ref.IsFolder {
			continue
		}
		if was := findFavoriteTreeNode(previous, ref.FavoriteID); was != nil {
			setNavFold(child, was.IsExpanded())
		}
	}
}

func (a *App) forgetNavNodeLabels(node *tview.TreeNode) {
	if node == nil || a.navNodeLabels == nil {
		return
	}
	delete(a.navNodeLabels, node)
	for _, child := range node.GetChildren() {
		a.forgetNavNodeLabels(child)
	}
}

func findFavoriteTreeNode(node *tview.TreeNode, favoriteID string) *tview.TreeNode {
	if node == nil {
		return nil
	}
	if ref, ok := node.GetReference().(*NavigationNode); ok && ref.FavoriteID == favoriteID {
		return node
	}
	for _, child := range node.GetChildren() {
		if found := findFavoriteTreeNode(child, favoriteID); found != nil {
			return found
		}
	}
	return nil
}

func firstSelectableChild(node *tview.TreeNode) *tview.TreeNode {
	if node == nil {
		return nil
	}
	for _, child := range node.GetChildren() {
		if child.GetReference() != nil {
			return child
		}
		if found := firstSelectableChild(child); found != nil {
			return found
		}
	}
	return nil
}

func treeContains(root, target *tview.TreeNode) bool {
	if root == nil || target == nil {
		return false
	}
	if root == target {
		return true
	}
	for _, child := range root.GetChildren() {
		if treeContains(child, target) {
			return true
		}
	}
	return false
}
