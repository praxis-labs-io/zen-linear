package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// padNavigationTree truncates and pads every node label to the tree's inner
// width, so long labels end in an ellipsis instead of clipping at the border
// and the selection highlight spans the full row (tview trees highlight only
// the label text). Original labels are cached so re-padding stays idempotent
// across redraws and resizes.
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
	original, ok := a.navNodeOriginalText[node]
	if !ok {
		original = node.GetText()
		a.navNodeOriginalText[node] = original
	}
	// Each tree level indents its label by two graphics columns; keep one
	// spare cell so the ellipsis lands before tview's own hard clip.
	if available := width - 2*level - 1; available > 0 {
		node.SetText(fitToWidth(original, available))
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
