package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type NavigationNode struct {
	ID           string
	Text         string
	TeamID       string
	Children     []*NavigationNode
	IsTeam       bool
	IsProject    bool
	IsStatus     bool
	IsCycle      bool
	IsIssue      bool
	StateID      string
	StateName    string
	CycleID      string
	CycleName    string
	IssueID      string
	CustomViewID string
	StateType    string
	IsFolder     bool
	// A fetch answered; row count cannot say so, since a team always holds its own All Issues row.
	ChildrenLoaded   bool
	IsGroup          bool
	FavoriteID       string
	FavoriteParentID string
}

func (a *App) buildNavigationTree() *tview.TreeView {
	tree := tview.NewTreeView()

	root := a.buildWaitingNavigationRoot()

	tree.SetBorder(false)
	tree.SetBackgroundColor(a.theme.Background)
	tree.SetGraphics(false)
	tree.SetFocusFunc(func() { a.claimNavFocus(false) })
	tree.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		a.padNavigationTree(width)
		return x, y, width, height
	})
	tree.SetRoot(root)
	tree.SetCurrentNode(root)
	tree.SetTopLevel(1)

	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		ref := node.GetReference()
		if ref != nil {
			if navNode, ok := ref.(*NavigationNode); ok {
				if navNode.IsFolder || navNode.IsGroup {
					setNavFold(node, !node.IsExpanded())
					return
				}
				if navNode.IsTeam {
					a.onTeamExpanded(navNode.TeamID, node)
					return
				}
				a.onNavigationSelected(navNode)
			}
		}
	})

	return tree
}

const (
	navIconOpen   = "\uf07c "
	navIconClosed = "\uf07b "
	navIconBranch = "\u2570 "
	navIconBlank  = "  "
)

func setNavFold(node *tview.TreeNode, expanded bool) {
	node.SetExpanded(expanded)
}

func (a *App) navRowColor(nav *NavigationNode) tcell.Color {
	if navIsFoldable(nav) {
		return a.theme.SecondaryText
	}
	return a.theme.Foreground
}

func navIsFoldable(nav *NavigationNode) bool {
	return nav.IsTeam || nav.IsGroup || nav.IsFolder
}

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
