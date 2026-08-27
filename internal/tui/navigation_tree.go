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
	// ChildrenLoaded says a fetch has built this team's rows. Counting the
	// rows cannot say it: a team holds its own All Issues either way, and a
	// load that came back with nothing would never be retried.
	ChildrenLoaded bool
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
	// No nesting lines. Depth is carried by the icon column each level indents
	// by, which says what a row is as well as where it sits.
	tree.SetGraphics(false)
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
				// A team only opens and closes, favorited or not. Its own All
				// Issues row is what scopes the list to the whole team, so
				// folding never costs a fetch and the rule has no exception.
				if navNode.IsTeam {
					a.onTeamExpanded(navNode.TeamID, node)
					return
				}
				// Update selection and refresh issues. Focus stays in the
				// navigation pane so the next pick is one keypress away.
				a.onNavigationSelected(navNode)
			}
		}
	})

	return tree
}

// The column every row begins with. A folder says the row opens and closes and
// which way it is, a branch says it is one of the rows a selection lands on,
// and the tree's own top row takes the same width blank. The two folders are
// Nerd Font glyphs, which the terminal's font has to carry; the branch is plain
// box drawing.
const (
	navIconOpen   = "\uf07c "
	navIconClosed = "\uf07b "
	navIconBranch = "\u2570 "
	navIconBlank  = "  "
)

// setNavFold opens or closes a row. Its icon is drawn from that state by
// padNavigationTree, so nothing here relabels it.
func setNavFold(node *tview.TreeNode, expanded bool) {
	node.SetExpanded(expanded)
}

// navRowColor mutes a row that does nothing but open and close. Those are the
// tree's structure, and their folder already says so, which leaves the color
// free to mark the rows a selection lands on.
func (a *App) navRowColor(nav *NavigationNode) tcell.Color {
	if navIsFoldable(nav) {
		return a.theme.SecondaryText
	}
	return a.theme.Foreground
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
