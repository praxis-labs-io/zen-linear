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
				// Folders only expand and collapse.
				if navNode.IsFolder {
					node.SetExpanded(!node.IsExpanded())
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
