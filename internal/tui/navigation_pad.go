package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// navNodeLabel caches a node's untruncated label alongside the inner width it
// was last fitted to. Both live in one entry so they cannot drift apart when
// the cache is cleared.
type navNodeLabel struct {
	original    string
	fittedWidth int
}

// padNavigationTree truncates and pads every node label to the tree's inner
// width, so long labels end in an ellipsis instead of clipping at the border
// and the selection highlight spans the full row (tview trees highlight only
// the label text). Original labels are cached so re-padding stays idempotent
// across redraws and resizes.
//
// This runs from SetDrawFunc, so it fires on every draw anywhere in the app.
// Nodes already fitted to the requested width are skipped: the rune-width
// measure and truncate is the expensive part, and re-deriving an identical
// label for every node on every keystroke is pure waste.
func (a *App) padNavigationTree(width int) {
	if a.navigationTree == nil || width <= 0 {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}
	a.padNavigationNode(root, 0, width)
}

func (a *App) padNavigationNode(node *tview.TreeNode, level int, width int) {
	label, cached := a.navNodeLabels[node]
	if !cached {
		label = navNodeLabel{original: node.GetText()}
		// Tighten tview's default indent of two to one, so each level advances
		// two cells (one graphics offset plus one indent).
		node.SetIndent(1)
	}
	available := width - 2*level
	needsFit := available > 0 && label.fittedWidth != available
	if needsFit {
		node.SetText(fitToWidth(label.original, available))
		label.fittedWidth = available
	}
	if !cached || needsFit {
		a.navNodeLabels[node] = label
	}

	for _, child := range node.GetChildren() {
		a.padNavigationNode(child, level+1, width)
	}
}

// fitToWidth truncates text to the given cell width with an ellipsis, or pads
// it with spaces to exactly that width.
func fitToWidth(text string, width int) string {
	textWidth := runewidth.StringWidth(text)
	if textWidth > width {
		return runewidth.Truncate(text, width, "…")
	}
	return text + strings.Repeat(" ", width-textWidth)
}
