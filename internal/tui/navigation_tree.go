package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NavigationNode represents a node in the navigation tree.
type NavigationNode struct {
	ID        string
	Text      string
	TeamID    string // For team, project, and status nodes
	Children  []*NavigationNode
	IsTeam    bool
	IsProject bool
	IsStatus  bool
	IsCycle   bool
	IsIssue   bool
	StateID   string
	StateName string
	CycleID   string
	CycleName string
	IssueID   string
	// CustomViewID makes the node show a Linear custom view's issues.
	CustomViewID string
	// StateType filters by workflow state type (e.g. triage), scoped to
	// TeamID when set.
	StateType string
	// IsFolder marks a favorites folder; selecting it toggles expansion.
	IsFolder bool
	// IsGroup marks one of a team's Cycles, Status and Projects headings.
	// Selecting it toggles expansion; it scopes nothing on its own.
	IsGroup bool
	// FavoriteID is set on nodes built from a favorite, so the toggle knows
	// the node is already favorited and the reorder knows what to move.
	FavoriteID string
	// FavoriteParentID is the enclosing favorite folder, for sibling lookup.
	FavoriteParentID string
}

// buildNavigationTree creates and configures the navigation tree widget. It is
// borderless: navigationPanel wraps it with the query box under one border, and
// that panel's border padding supplies the gutter the tree used to inset for
// itself.
func (a *App) buildNavigationTree() *tview.TreeView {
	tree := tview.NewTreeView()

	root := a.buildWaitingNavigationRoot()

	tree.SetBorder(false)
	tree.SetBackgroundColor(a.theme.Background)
	// The other half of the pane's click handling; see claimNavFocus. Without
	// it a click on the tree leaves the query box holding the keys, and Esc
	// there wipes a query the user never went back to.
	tree.SetFocusFunc(func() { a.claimNavFocus(false) })
	tree.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		// Re-fit node labels on every draw so they track pane resizes and
		// lazily added nodes.
		a.padNavigationTree(width)
		return x, y, width, height
	})
	tree.SetRoot(root)
	tree.SetCurrentNode(root)
	// The pane border names the workspace, so the root row is hidden and its
	// children are the top level.
	tree.SetTopLevel(1)

	// Handle selection for all nodes (teams, projects, and "All Issues")
	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		ref := node.GetReference()
		if ref != nil {
			if navNode, ok := ref.(*NavigationNode); ok {
				// Folders and a team's groups only expand and collapse.
				if navNode.IsFolder || navNode.IsGroup {
					setNavFold(node, !node.IsExpanded())
					return
				}
				// For team nodes, handle expand/collapse
				if navNode.IsTeam {
					a.onTeamExpanded(navNode.TeamID, node)
				}
				// Update selection and refresh issues. Focus stays in the
				// navigation pane so the next pick is one keypress away.
				a.onNavigationSelected(navNode)
			}
		}
	})

	return tree
}

// The glyphs marking a row that opens and closes. tview draws no expander of
// its own and its per-level prefixes cannot vary with a node's state, so the
// row's own label carries it.
const (
	navFoldOpen   = "▾ "
	navFoldClosed = "▸ "
)

// navFoldLabel is a foldable row's text, marked for the state it is in.
func navFoldLabel(text string, expanded bool) string {
	if expanded {
		return navFoldOpen + text
	}
	return navFoldClosed + text
}

// setNavFold opens or closes a row and re-marks it. The glyph is part of the
// label, so a toggle that skips this leaves the row claiming the state it left.
// padNavigationTree re-fits anything relabelled here on the next draw.
func setNavFold(node *tview.TreeNode, expanded bool) {
	node.SetExpanded(expanded)
	if nav, ok := node.GetReference().(*NavigationNode); ok && navIsFoldable(nav) {
		node.SetText(navFoldLabel(nav.Text, expanded))
	}
}

// navIsFoldable reports whether a row opens and closes rather than scoping the
// issue list.
func navIsFoldable(nav *NavigationNode) bool {
	return nav.IsTeam || nav.IsGroup || nav.IsFolder
}

// revealNavNode opens every row above a node so a restored selection lands on
// one that is drawn. A team's groups start folded, so expanding the team alone
// leaves the cursor on a row nobody can see.
func revealNavNode(ancestor, target *tview.TreeNode) bool {
	if ancestor == target {
		return true
	}
	for _, child := range ancestor.GetChildren() {
		if revealNavNode(child, target) {
			setNavFold(ancestor, true)
			return true
		}
	}
	return false
}
