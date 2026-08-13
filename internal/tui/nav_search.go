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

	a.navSearchInput = newThemedInputField(a.theme.InputBg)
	a.navSearchInput.
		SetLabel("/ ").
		SetLabelColor(a.theme.Accent).
		SetFieldWidth(0).
		// The pane is around twenty columns wide, so the placeholder is one
		// word or it is truncated.
		SetPlaceholder("Search").
		SetFieldStyle(tcell.StyleDefault.Foreground(a.theme.Foreground).Background(a.theme.InputBg)).
		SetPlaceholderStyle(tcell.StyleDefault.Foreground(a.theme.SecondaryText).Background(a.theme.InputBg)).
		SetBackgroundColor(a.theme.Background)
	// Restore the query before installing the change handler so a theme
	// rebuild does not re-fire the search.
	a.navSearchInput.SetText(previousQuery)
	a.navSearchInput.SetChangedFunc(func(text string) {
		a.searchQuery = text
		a.scheduleSearchDebounce(text)
	})

	a.navSearchRule = tview.NewBox()
	a.navSearchRule.SetBackgroundColor(a.theme.Background)
	a.navSearchRule.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		style := tcell.StyleDefault.Foreground(a.theme.Border).Background(a.theme.Background)
		for column := range width {
			screen.SetContent(x+column, y, tcell.RuneHLine, nil, style)
		}
		return x, y, 0, 0
	})

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
	a.navigationPanel.
		AddItem(a.navSearchInput, 1, 0, false).
		AddItem(a.navSearchRule, 1, 0, false).
		AddItem(a.navigationTree, 0, 1, false)

	a.applyNavSearchStyles()
}

// applyNavSearchStyles lights the query box while it holds the keyboard and
// mutes it otherwise. The tree's selection stays lit either way: it names the
// list the issues pane is showing, which is still true while you type.
func (a *App) applyNavSearchStyles() {
	if a.navSearchInput == nil {
		return
	}
	label, text := a.theme.SecondaryText, a.theme.SecondaryText
	if a.navSearchActive() {
		label, text = a.theme.Accent, a.theme.Foreground
	}
	a.navSearchInput.SetLabelColor(label)
	a.navSearchInput.SetFieldStyle(tcell.StyleDefault.
		Foreground(text).
		Background(a.theme.InputBg))
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
