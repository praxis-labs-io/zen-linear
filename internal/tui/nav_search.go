package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) buildNavigationPanel() {
	previousQuery := a.searchQuery
	if a.navSearchInput != nil {
		previousQuery = a.navSearchInput.GetText()
	}

	a.navSearchInput = tview.NewInputField().
		SetLabel("/ ").
		SetLabelColor(a.theme.Accent).
		SetFieldWidth(0).
		SetPlaceholder("Search").
		SetFieldBackgroundColor(a.theme.Background).
		SetFieldTextColor(a.theme.Foreground).
		SetPlaceholderTextColor(a.theme.SecondaryText)
	a.navSearchInput.SetBackgroundColor(a.theme.Background)
	a.navSearchInput.SetText(previousQuery)
	a.navSearchInput.SetChangedFunc(func(text string) {
		a.searchQuery = text
		a.scheduleSearchDebounce(text)
	})
	a.navSearchInput.SetFocusFunc(func() { a.claimNavFocus(true) })

	a.navSearchFrame = tview.NewFlex().SetDirection(tview.FlexRow)
	a.navSearchFrame.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.navSearchFrame.
		SetBorder(true).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	a.navSearchFrame.AddItem(a.navSearchInput, 0, 1, true)

	a.navigationPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	a.navigationPanel.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.navigationPanel.
		SetBorder(true).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	a.navigationPanel.SetBorderPadding(0, 0, 1, 1)
	a.navigationPanel.
		AddItem(a.navSearchFrame, 3, 0, a.navSearchFocused).
		AddItem(a.navigationTree, 0, 1, !a.navSearchFocused)

	a.applyNavSearchStyles()
}

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

func (a *App) navSelectionIsLit() bool {
	if a.activeIssuesSection != IssuesSectionSearch {
		return true
	}
	return a.focusedPane == FocusNavigation && !a.navSearchFocused
}

func (a *App) applyNavSelectionStyle(node *tview.TreeNode) {
	if node == nil {
		return
	}
	style := selectionStyle(a.theme)
	if !a.navSelectionIsLit() {
		style = tcell.StyleDefault.Foreground(node.GetColor()).Background(a.theme.Background)
	}
	node.SetSelectedTextStyle(style)
	for _, child := range node.GetChildren() {
		a.applyNavSelectionStyle(child)
	}
}

func (a *App) navSearchActive() bool {
	return a.focusedPane == FocusNavigation && a.navSearchFocused
}

func (a *App) claimNavFocus(searchBox bool) {
	if a.focusedPane == FocusPalette || a.activeModal() != nil {
		return
	}
	a.navSearchFocused = searchBox
	a.applyNavSearchStyles()
	a.updateStatusBar()
}

func (a *App) focusNavSearch() {
	a.navSearchFocused = true
	a.focusPane(FocusNavigation)
}

func (a *App) focusNavigationTree() {
	a.navSearchFocused = false
	a.updateFocus()
}

func (a *App) clearNavSearch() {
	if a.navSearchInput != nil {
		a.navSearchInput.SetText("")
	}
	a.cancelSearchDebounce()
	a.clearSearchResults()
	a.searchQuery = ""
}

func (a *App) handleNavSearchKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.quit()
		return nil
	case tcell.KeyEscape:
		if a.navSearchInput.GetText() != "" {
			a.clearNavSearch()
			a.jumpToSection(IssuesSectionList, 0)
			return nil
		}
		a.focusNavigationTree()
		return nil
	case tcell.KeyDown, tcell.KeyTab:
		a.focusNavigationTree()
		return nil
	case tcell.KeyBacktab:
		return nil
	case tcell.KeyEnter:
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
