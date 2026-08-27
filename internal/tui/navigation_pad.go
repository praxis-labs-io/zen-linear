package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// navNodeLabel caches a node's untruncated label alongside the fitted text and
// width last written to it. All three live in one entry so they cannot drift
// apart when the cache is cleared.
type navNodeLabel struct {
	original    string
	fitted      string
	fittedWidth int
	// fittedPrefix is the indent and column the row was last drawn with. A
	// toggle changes no text, so without this the row would keep the icon it
	// had.
	fittedPrefix string
}

// padNavigationTree truncates and pads every node label to the tree's inner
// width, so long labels end in an ellipsis instead of clipping at the border
// and the selection highlight spans the full row (tview trees highlight only
// the label text). Original labels are cached so re-padding stays idempotent
// across redraws and resizes.
//
// This runs from SetDrawFunc, so it fires on every draw anywhere in the app.
// Nodes still carrying the text they were last fitted to, at the same width,
// are skipped: the rune-width measure and truncate is the expensive part, and
// re-deriving an identical label for every node on every keystroke is pure
// waste. The check is against the node's current text, not the cache alone, so
// anything that relabels a node elsewhere is picked up on the next draw rather
// than silently discarded.
func (a *App) padNavigationTree(width int) {
	if a.navigationTree == nil || width <= 0 {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}
	// The root is hidden, so its children are the rows drawn at level 0.
	for _, child := range root.GetChildren() {
		a.padNavigationNode(child, 0, width, false)
	}
}

func (a *App) padNavigationNode(node *tview.TreeNode, level, width int, owned bool) {
	label, cached := a.navNodeLabels[node]
	text := node.GetText()
	if !cached {
		// Every row's label starts at the pane's left edge, so the cursor line
		// runs the full width rather than beginning where the text does. The
		// indent is written into the label instead.
		node.SetIndent(0)
	}
	if !cached || text != label.fitted {
		// Either the node is new or something relabelled it; the text on it now
		// is the truth to fit from.
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

	// A section heading is a label over the rows beneath it rather than a level
	// of its own, so those rows begin at the edge with it.
	childLevel := level + 1
	if isNavSectionHeading(node) {
		childLevel = level
	}
	// Everything under the tree's own top row belongs to something, whether
	// that is a heading, a team or a folder.
	for _, child := range node.GetChildren() {
		a.padNavigationNode(child, childLevel, width, true)
	}
}

// navRowPrefix is the indent a row carries and the column it begins with: a
// folder where the row opens and closes, and a branch where it is one of the
// rows a selection can land on, so every title lines up with its siblings and
// sits a column in from whatever it hangs off.
func navRowPrefix(node *tview.TreeNode, level int, owned bool) string {
	// Two cells a level, the width of the column each row begins with, so a
	// row sits under the title of the one it hangs off.
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

// isNavSectionHeading reports whether a row is one of the tree's section labels
// rather than something the cursor can rest on.
func isNavSectionHeading(node *tview.TreeNode) bool {
	_, isNav := node.GetReference().(*NavigationNode)
	return !isNav && len(node.GetChildren()) > 0
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
