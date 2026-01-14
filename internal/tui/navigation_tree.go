package tui

import (
	"github.com/rivo/tview"
)

// NavigationNode represents a node in the navigation tree.
type NavigationNode struct {
	ID        string
	Text      string
	TeamID    string // For team and project nodes
	Children  []*NavigationNode
	IsTeam    bool
	IsProject bool
}

// buildNavigationTree creates and configures the navigation tree widget.
func (a *App) buildNavigationTree() *tview.TreeView {
	tree := tview.NewTreeView()

	// Create initial root with "Loading..." placeholder
	root := tview.NewTreeNode("Linear").
		SetColor(LinearTheme.Accent).
		SetSelectable(false)

	loadingNode := tview.NewTreeNode("Loading teams...").
		SetColor(LinearTheme.SecondaryText).
		SetSelectable(false)
	root.AddChild(loadingNode)

	tree.SetBorder(true).
		SetTitle(" Navigation ").
		SetTitleColor(LinearTheme.Foreground).
		SetBorderColor(LinearTheme.Border)
	tree.SetBackgroundColor(LinearTheme.Background)
	tree.SetRoot(root)
	tree.SetCurrentNode(root)

	// Handle selection for all nodes (teams, projects, and "All Issues")
	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		ref := node.GetReference()
		if ref != nil {
			if navNode, ok := ref.(*NavigationNode); ok {
				// For team nodes, handle expand/collapse
				if navNode.IsTeam {
					a.onTeamExpanded(navNode.TeamID, node)
				}
				// Update selection and refresh issues
				a.onNavigationSelected(navNode)
			}
		}
	})

	return tree
}
