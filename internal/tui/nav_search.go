package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The navigation pane is a query box over the tree, under one border. The
// search it drives is workspace-wide: it takes neither the tree's scope nor the
// rich filters, and its results replace the list in the issues pane until the
// query is emptied.

// buildNavigationPanel (re)creates the navigation pane's shell and its query
// box. Called from buildLayout and again on theme changes: tview bakes the
// input's inner background at construction, so re-theming needs a fresh
// InputField. The tree is reused, never rebuilt, since it holds the nav state.
func (a *App) buildNavigationPanel() {
	previousQuery := a.searchQuery
	if a.navSearchInput != nil {
		previousQuery = a.navSearchInput.GetText()
	}

	// A form field, not a filled input: the frame around it is what says where
	// it starts and ends, and the field itself takes the pane's background so a
	// transparent theme stays transparent.
	a.navSearchInput = tview.NewInputField().
		SetLabel("/ ").
		SetLabelColor(a.theme.Accent).
		SetFieldWidth(0).
		// The pane is around twenty columns wide, so the placeholder is one
		// word or it is truncated.
		SetPlaceholder("Search").
		SetFieldBackgroundColor(a.theme.Background).
		SetFieldTextColor(a.theme.Foreground).
		SetPlaceholderTextColor(a.theme.SecondaryText)
	a.navSearchInput.SetBackgroundColor(a.theme.Background)
	// Restore the query before installing the change handler so a theme
	// rebuild does not re-fire the search.
	a.navSearchInput.SetText(previousQuery)
	a.navSearchInput.SetChangedFunc(func(text string) {
		a.searchQuery = text
		a.scheduleSearchDebounce(text)
	})

	a.navSearchFrame = tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the
	// fill so the layer beneath cannot bleed through.
	a.navSearchFrame.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.navSearchFrame.
		SetBorder(true).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	a.navSearchFrame.AddItem(a.navSearchInput, 0, 1, true)

	a.navigationPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the
	// fill so the layer beneath cannot bleed through.
	a.navigationPanel.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.navigationPanel.
		SetBorder(true).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	// One column of gutter on each side, for the box, the rule and the tree
	// alike. The tree has no root row and no graphics on its top level, so
	// without this its first column would sit on the border.
	a.navigationPanel.SetBorderPadding(0, 0, 1, 1)
	// One of the two has to carry the Flex's focus flag. tview delegates focus
	// down the tree on SetRoot and on any page it adds, and a Flex with nothing
	// flagged keeps the focus on its own Box, which answers no keys: the pane
	// looks focused and the arrows do nothing until something calls
	// updateFocus again.
	a.navigationPanel.
		AddItem(a.navSearchFrame, 3, 0, a.navSearchFocused).
		AddItem(a.navigationTree, 0, 1, !a.navSearchFocused)

	a.applyNavSearchStyles()
}

// applyNavSearchStyles lights the query box while it holds the keyboard, and
// puts out the tree's selection while search results are what the issues pane
// is showing. A lit row there claims to name the list on screen, and search
// takes no list.
func (a *App) applyNavSearchStyles() {
	if a.navSearchInput == nil || a.navSearchFrame == nil {
		return
	}
	border, label := a.theme.Border, a.theme.SecondaryText
	if a.navSearchActive() {
		border, label = a.theme.BorderFocus, a.theme.Accent
	}
	a.navSearchFrame.SetBorderColor(border)
	a.navSearchInput.SetLabelColor(label)
	a.applyNavSelectionStyle(a.navigationTree.GetRoot())
}

// applyNavSelectionStyle sets how the tree's current row paints. Every node
// gets it, not just the current one, so arrowing around while results are up
// cannot light the row the cursor lands on.
func (a *App) applyNavSelectionStyle(node *tview.TreeNode) {
	if node == nil {
		return
	}
	style := selectionStyle(a.theme)
	if a.activeIssuesSection == IssuesSectionSearch {
		style = tcell.StyleDefault.Foreground(node.GetColor()).Background(a.theme.Background)
	}
	node.SetSelectedTextStyle(style)
	for _, child := range node.GetChildren() {
		a.applyNavSelectionStyle(child)
	}
}

// navSearchActive reports whether typed keys belong to the query box.
func (a *App) navSearchActive() bool {
	return a.focusedPane == FocusNavigation && a.navSearchFocused
}

// focusNavSearch puts the keyboard in the query box, revealing the navigation
// pane first when it has been toggled off or dropped by a narrow layout.
func (a *App) focusNavSearch() {
	a.navSearchFocused = true
	a.focusPane(FocusNavigation)
}

// focusNavigationTree hands the keyboard from the query box to the tree,
// leaving the query where it is.
func (a *App) focusNavigationTree() {
	a.navSearchFocused = false
	a.updateFocus()
}

// navigationTreeIsAtTop reports whether the tree's cursor is on its first row,
// the one an Up steps off. The root is hidden, so that row is its first child.
func (a *App) navigationTreeIsAtTop() bool {
	root := a.navigationTree.GetRoot()
	if root == nil {
		return false
	}
	children := root.GetChildren()
	return len(children) > 0 && a.navigationTree.GetCurrentNode() == children[0]
}

// clearNavSearch empties the query and drops the results. Emptying the box
// schedules a search for the empty string, so cancel that after and do the work
// here rather than a debounce window later. What the issues pane mounts next is
// the caller's, since the two callers want different things there.
func (a *App) clearNavSearch() {
	if a.navSearchInput != nil {
		a.navSearchInput.SetText("")
	}
	a.cancelSearchDebounce()
	a.clearSearchResults()
	a.searchQuery = ""
}

// handleNavSearchKey routes keys while the query box has focus. Anything not
// handled here falls through to the InputField, so plain letters type instead
// of firing global or pane shortcuts.
func (a *App) handleNavSearchKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyEscape:
		if a.navSearchInput.GetText() != "" {
			// First Esc clears the query and brings the list back; a second
			// hands the keyboard down to the tree.
			a.clearNavSearch()
			a.jumpToSection(IssuesSectionList, 0)
			return nil
		}
		a.focusNavigationTree()
		return nil
	case tcell.KeyDown, tcell.KeyTab:
		// The tree is the pane's other control, and Down is a plain move into
		// it. Enter is what crosses to the results.
		a.focusNavigationTree()
		return nil
	case tcell.KeyBacktab:
		// The box is the pane's first control, so there is nothing behind it.
		return nil
	case tcell.KeyEnter:
		// With no results there is nothing mounted to hand the keyboard to, and
		// moving focus anyway would leave the pane dead.
		if len(a.searchIssueRows) == 0 {
			return nil
		}
		row, _ := a.searchResultsTable.GetSelection()
		if row < 1 || row > len(a.searchIssueRows) {
			row = 1
		}
		a.navSearchFocused = false
		a.focusedPane = FocusIssues
		a.searchResultsTable.Select(row, 0)
		if issue := a.getIssueFromRowForSection(row, IssuesSectionSearch); issue != nil {
			a.selectIssueNow(*issue)
		}
		a.updateFocus()
		return nil
	}
	return event
}
