package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

type navNodeLabel struct {
	original     string
	fitted       string
	fittedWidth  int
	fittedPrefix string
}

func (a *App) padNavigationTree(width int) {
	if a.navigationTree == nil || width <= 0 {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}
	for _, child := range root.GetChildren() {
		a.padNavigationNode(child, 0, width, false)
	}
}

func (a *App) padNavigationNode(node *tview.TreeNode, level, width int, owned bool) {
	label, cached := a.navNodeLabels[node]
	text := node.GetText()
	if !cached {
		node.SetIndent(0)
	}
	if !cached || text != label.fitted {
		label = navNodeLabel{original: text}
	}

	prefix := navRowPrefix(node, level, owned)
	needsFit := label.fittedWidth != width || label.fitted != text || label.fittedPrefix != prefix
	if needsFit {
		label.fitted = fitToWidth(prefix+label.original, width)
		label.fittedWidth = width
		label.fittedPrefix = prefix
		node.SetText(label.fitted)
	}
	if !cached || needsFit {
		a.navNodeLabels[node] = label
	}

	childLevel := level + 1
	if isNavSectionHeading(node) {
		childLevel = level
	}
	childOwned := false
	if nav, ok := node.GetReference().(*NavigationNode); ok {
		childOwned = navIsFoldable(nav)
	}
	for _, child := range node.GetChildren() {
		a.padNavigationNode(child, childLevel, width, childOwned)
	}
}

func navRowPrefix(node *tview.TreeNode, level int, owned bool) string {
	indent := strings.Repeat("  ", level)
	nav, ok := node.GetReference().(*NavigationNode)
	if !ok {
		return indent
	}
	switch {
	case !navIsFoldable(nav):
		if !owned {
			return indent + navIconBlank
		}
		return indent + navIconBranch
	case node.IsExpanded():
		return indent + navIconOpen
	default:
		return indent + navIconClosed
	}
}

func isNavSectionHeading(node *tview.TreeNode) bool {
	_, isNav := node.GetReference().(*NavigationNode)
	return !isNav && len(node.GetChildren()) > 0
}

// go-runewidth's default counts East Asian Ambiguous runes, which the folder and branch glyphs are, as two cells under a CJK locale.
var navCellWidth = &runewidth.Condition{EastAsianWidth: false}

func fitToWidth(text string, width int) string {
	textWidth := navCellWidth.StringWidth(text)
	if textWidth > width {
		return navCellWidth.Truncate(text, width, "…")
	}
	return text + strings.Repeat(" ", width-textWidth)
}
