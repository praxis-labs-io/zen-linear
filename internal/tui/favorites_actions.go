package tui

import (
	"context"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

// favoriteTargetForNode maps a navigation node onto the Linear entity a
// favorite would point at. Custom views and predefined views are tested before
// the entity booleans, the same order refreshIssuesWithFocusChange uses to turn
// a node into query params. Linear has no favorite type for a workflow state,
// so status nodes report false.
func favoriteTargetForNode(node *NavigationNode) (linearapi.FavoriteTarget, bool) {
	if node == nil || node.IsFolder {
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

// favoriteMatchesTarget reports whether an existing favorite already points at
// the target entity.
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

// favoriteForNode finds the favorite a navigation node stands for. Nodes in the
// Favorites section carry the id outright; everything else matches on the
// entity, so pressing the key on an already favorited project removes it
// instead of adding a second one.
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

// currentNavigationNode returns the node under the navigation cursor.
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

// handleToggleFavorite favorites or unfavorites the navigation item under the
// cursor.
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

// addFavorite creates a favorite and folds it into the rendered section.
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

// removeFavorite deletes a favorite and drops it from the rendered section.
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

// favoriteReorder is the pair of sort-order writes that moves a favorite one
// slot among its siblings.
type favoriteReorder struct {
	MovedID      string
	MovedSort    float64
	NeighborID   string
	NeighborSort float64
}

// planFavoriteReorder works out the swap that moves a favorite one slot.
// delta is -1 for up and 1 for down; it reports false at either end of the
// sibling list. Swapping the two sort orders reuses values Linear already
// assigned, so repeated moves cannot drift the way interpolation does.
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

// favoriteMove is a reparenting write: where a favorite lands and at what
// position.
type favoriteMove struct {
	FavoriteID string
	ParentID   string
	SortOrder  float64
}

// planFavoriteNest works out the move that tucks a favorite into the folder
// directly above it, the way an outliner indents under the previous sibling. It
// reports false when there is no folder above it, or when the favorite is
// itself a folder: Linear's sidebar does not nest folders.
func planFavoriteNest(favorites []linearapi.Favorite, favoriteID, parentID string) (favoriteMove, bool) {
	siblings := favoriteSiblings(favorites, parentID)
	index := indexOfFavorite(siblings, favoriteID)
	if index < 1 {
		return favoriteMove{}, false
	}
	if siblings[index].Type == favoriteTypeFolder {
		return favoriteMove{}, false
	}
	folder := siblings[index-1]
	if folder.Type != favoriteTypeFolder {
		return favoriteMove{}, false
	}

	// Land at the bottom of the folder, where an indent naturally puts it.
	sortOrder := folder.SortOrder + 1
	if children := favoriteSiblings(favorites, folder.ID); len(children) > 0 {
		sortOrder = children[len(children)-1].SortOrder + 1
	}
	return favoriteMove{FavoriteID: favoriteID, ParentID: folder.ID, SortOrder: sortOrder}, true
}

// planFavoriteUnnest works out the move that lifts a favorite out of its
// folder, landing it just below that folder. It reports false at the top level.
func planFavoriteUnnest(favorites []linearapi.Favorite, favoriteID, parentID string) (favoriteMove, bool) {
	if parentID == "" {
		return favoriteMove{}, false
	}
	folder, ok := favoriteByID(favorites, parentID)
	if !ok {
		return favoriteMove{}, false
	}

	grandparent := favoriteParentIDs(favorites)[folder.ID]
	outer := favoriteSiblings(favorites, grandparent)
	folderIndex := indexOfFavorite(outer, folder.ID)

	// Slot between the folder and whatever follows it, so the favorite appears
	// where it was dragged out to.
	sortOrder := folder.SortOrder + 1
	if folderIndex >= 0 && folderIndex+1 < len(outer) {
		sortOrder = (folder.SortOrder + outer[folderIndex+1].SortOrder) / 2
	}
	return favoriteMove{FavoriteID: favoriteID, ParentID: grandparent, SortOrder: sortOrder}, true
}

// nestFavorite moves a favorite into the folder above it, or out of its folder
// when out is true. It reports whether it consumed the key.
func (a *App) nestFavorite(node *NavigationNode, out bool) bool {
	if node == nil || node.FavoriteID == "" {
		return false
	}

	plan, ok := planFavoriteNest(a.favorites, node.FavoriteID, node.FavoriteParentID)
	if out {
		plan, ok = planFavoriteUnnest(a.favorites, node.FavoriteID, node.FavoriteParentID)
	}
	if !ok {
		return false
	}

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
	return true
}

// applyFavoriteMove records a reparent locally.
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

// indexOfFavorite returns the position of a favorite in a slice, or -1.
func indexOfFavorite(favorites []linearapi.Favorite, favoriteID string) int {
	for i, favorite := range favorites {
		if favorite.ID == favoriteID {
			return i
		}
	}
	return -1
}

// favoriteByID looks a favorite up by id.
func favoriteByID(favorites []linearapi.Favorite, favoriteID string) (linearapi.Favorite, bool) {
	if i := indexOfFavorite(favorites, favoriteID); i >= 0 {
		return favorites[i], true
	}
	return linearapi.Favorite{}, false
}

// moveFavorite swaps a favorite with its neighbor in Linear's sort order.
// delta is -1 for up and 1 for down. It reports whether it consumed the key:
// on anything but a favorite, and at either end of the list, the key belongs to
// the tree rather than raising an error at someone who pressed it in passing.
func (a *App) moveFavorite(node *NavigationNode, delta int) bool {
	if node == nil || node.FavoriteID == "" {
		return false
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
			// The first write landed, so the pair now shares a sort order.
			// Linear keeps a stable order and the next move repairs it.
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

// runFavoriteAction runs a background favorites write, reporting a failure
// through reportFavoriteError and otherwise applying onSuccess on the UI thread.
// favoritesSettled always fires so tests can wait on the goroutine either way.
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

// reportFavoriteError logs a failed favorites mutation and surfaces it.
func (a *App) reportFavoriteError(err error, format string, args ...interface{}) {
	logger.ErrorWithErr(err, "tui.favorites: "+format, args...)
	a.QueueUpdateDraw(func() {
		a.updateStatusBarWithError(err)
		a.favoritesSettled()
	})
}

// favoritesSettled signals that a favorites mutation finished, for tests that
// need to wait on the goroutine.
func (a *App) favoritesSettled() {
	if a.favoritesChanged != nil {
		a.favoritesChanged()
	}
}

// favoriteSiblings returns the renderable favorites sharing a parent folder, in
// display order. Favorite types the tree drops are excluded, so a move never
// swaps past something invisible.
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

// upsertFavorite replaces a favorite with the same id, or appends it.
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

// removeFavoriteByID drops a favorite, along with anything nested inside it
// when it is a folder.
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

// applyFavoriteSortOrders writes new sort orders and re-sorts.
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

// favoriteLabel names a favorite for the status bar.
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

// refreshFavoritesSection rebuilds only the Favorites group. The full
// rebuildNavigationTree drops team expansion and resets the cursor to All
// Issues, which a toggle or a reorder has no business doing.
//
// preferFavoriteID, when it resolves, takes the cursor, so a reorder follows the
// item it moved. Otherwise the cursor stays where it was, unless the node under
// it is the one that just went away.
func (a *App) refreshFavoritesSection(preferFavoriteID string) {
	if a.navigationTree == nil {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}

	previous := a.favoritesGroup
	group := a.buildFavoritesGroup(a.favorites)
	if previous == nil && group == nil {
		return
	}

	children := root.GetChildren()
	rebuilt := make([]*tview.TreeNode, 0, len(children)+1)
	if previous != nil {
		for _, child := range children {
			if child == previous {
				if group != nil {
					rebuilt = append(rebuilt, group)
				}
				continue
			}
			rebuilt = append(rebuilt, child)
		}
		a.forgetNavNodeLabels(previous)
	} else {
		// No section yet: it belongs directly under "All Issues".
		at := min(1, len(children))
		rebuilt = append(rebuilt, children[:at]...)
		rebuilt = append(rebuilt, group)
		rebuilt = append(rebuilt, children[at:]...)
	}

	a.favoritesGroup = group
	root.SetChildren(rebuilt)
	a.applyNavSelectionStyle(root)
	a.restoreNavigationCursor(root, group, preferFavoriteID)
	// The disk copy is what the next launch paints, so a toggle or a reorder
	// has to reach it too, or the tree comes back wrong for a moment.
	a.recordNavCacheAsync()
}

// restoreNavigationCursor keeps the cursor on something that still exists after
// the Favorites group is replaced.
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

// forgetNavNodeLabels drops a subtree from the label cache, which is otherwise
// only cleared on a full rebuild.
func (a *App) forgetNavNodeLabels(node *tview.TreeNode) {
	if node == nil || a.navNodeLabels == nil {
		return
	}
	delete(a.navNodeLabels, node)
	for _, child := range node.GetChildren() {
		a.forgetNavNodeLabels(child)
	}
}

// findFavoriteTreeNode locates the rendered node for a favorite id.
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

// firstSelectableChild returns the first node a cursor may rest on.
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

// treeContains reports whether a node is still attached to the tree.
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
