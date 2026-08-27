package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) applyThemeAndDensity() {
	a.theme = ResolveTheme(a.config.Theme)
	a.themeTags = NewThemeTags(a.theme)
	a.density = ResolveDensity(a.config.Density)
	initMarkdownRenderer(a.theme)

	a.applyThemeStyles()
	a.applyThemeToComponents()
	a.applyDensityToComponents()
	a.rebuildModals()
	a.updateStatusBar()
	a.updateDetailsView()
	a.updatePaletteList()
}

func (a *App) applyThemeStyles() {
	tview.Styles.PrimitiveBackgroundColor = a.theme.Background
	tview.Styles.ContrastBackgroundColor = a.theme.Background
	tview.Styles.MoreContrastBackgroundColor = a.theme.HeaderBg
	tview.Styles.BorderColor = a.theme.Border
	tview.Styles.TitleColor = a.theme.Foreground
	tview.Styles.GraphicsColor = a.theme.Border
	tview.Styles.PrimaryTextColor = a.theme.Foreground
	tview.Styles.SecondaryTextColor = a.theme.SecondaryText
	tview.Styles.TertiaryTextColor = a.theme.SecondaryText
	tview.Styles.InverseTextColor = a.theme.InverseTextColor()
	tview.Styles.ContrastSecondaryTextColor = a.theme.SecondaryText

	// Square by default; the setting swaps in rounded corner runes. Both
	// branches assign so toggling the setting at runtime restores either look.
	if a.config.RoundedBorders {
		tview.Borders.TopLeft = '\u256d'     // ╭
		tview.Borders.TopRight = '\u256e'    // ╮
		tview.Borders.BottomLeft = '\u2570'  // ╰
		tview.Borders.BottomRight = '\u256f' // ╯
	} else {
		tview.Borders.TopLeft = tview.BoxDrawingsLightDownAndRight
		tview.Borders.TopRight = tview.BoxDrawingsLightDownAndLeft
		tview.Borders.BottomLeft = tview.BoxDrawingsLightUpAndRight
		tview.Borders.BottomRight = tview.BoxDrawingsLightUpAndLeft
	}

	// Focused panes are already highlighted via BorderFocus; keep single-line
	// borders instead of tview's default double-line focus runes.
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
}

func (a *App) applyThemeToComponents() {
	if a.navigationTree != nil {
		a.navigationTree.SetBackgroundColor(a.theme.Background)
		a.recolorNavigationTree()
	}
	if a.navigationPanel != nil {
		// Rebuild the shell so the query box picks up the new InputBg (tview
		// bakes it at construction), then remount it: contentFlex and the focus
		// are both still holding the old pointer.
		a.buildNavigationPanel()
		a.rebuildContentLayout()
	}

	if a.listIssuesTable != nil {
		a.applyIssuesTableTheme(a.listIssuesTable)
		renderIssuesTableModel(a.listIssuesTable, a.listIssueRows, a.listIDToIssue, a.selectedIssueID(IssuesSectionList), a.theme, a.issueColumns())
	}
	if a.searchResultsTable != nil {
		a.applyIssuesTableTheme(a.searchResultsTable)
		renderIssuesTableModel(a.searchResultsTable, a.searchIssueRows, a.searchIDToIssue, a.selectedIssueID(IssuesSectionSearch), a.theme, a.issueColumns())
	}
	if a.issuesPlaceholder != nil {
		// Rebuilt rather than restyled: the colors are baked into the flex and
		// its text view at construction.
		a.buildIssuesPlaceholder()
		a.updateIssuesColumnLayout()
	}

	// The background goes through TextView's own setter rather than the Box
	// one the chain would reach, so the text style tracks it. Left behind, the
	// two disagree and tview fills the inner rect, which the centered reading
	// measure shows as a block narrower than the pane.
	if a.detailsPageView != nil {
		a.detailsPageView.SetBackgroundColor(a.theme.Background)
	}
	if a.detailsView != nil {
		a.detailsView.SetTitleColor(a.theme.Foreground).
			SetBorderColor(a.theme.Border).
			SetBackgroundColor(a.theme.Background)
		a.applyComposeTheme()
	}

	if a.statusRow != nil {
		a.statusRow.SetBackgroundColor(a.theme.Background)
		style := tcell.StyleDefault.Background(a.theme.Background).Foreground(a.theme.SecondaryText)
		for _, view := range []*tview.TextView{a.statusBar, a.statusToast} {
			view.SetTextStyle(style).SetBackgroundColor(a.theme.Background)
		}
	}
}

func (a *App) applyDensityToComponents() {
	if a.detailsView != nil {
		padding := a.density.DetailsPadding
		a.detailsView.SetBorderPadding(0, 0, padding.Left, padding.Right)
	}
	if a.statusRow != nil {
		a.applyStatusBarPadding()
	}
}

func (a *App) rebuildModals() {
	if a.pages != nil {
		a.pages.RemovePage("palette")
	}
	a.paletteModal = a.buildPaletteModal()
	if a.pages != nil {
		a.pages.AddPage("palette", a.paletteModal, true, false)
	}

	a.pickerModal = NewPickerModal(a)
	a.issueFormModal = NewIssueFormModal(a)
	a.textInputModal = NewTextInputModal(a)
	a.multiSelectModal = NewMultiSelectModal(a)
	a.settingsModal = NewSettingsModal(a)
	a.promptTemplatesModal = NewAgentPromptTemplatesModal(a)
	a.agentPromptModal = NewAgentPromptModal(a)
	if a.pages == nil || !a.pages.HasPage("agent_output") {
		a.agentOutputModal = NewAgentOutputModal(a)
	} else {
		a.agentOutputModal.ApplyTheme(a.theme)
	}
	a.confirmationModal = NewConfirmationModal(a)
	a.keysModal = NewKeysModal(a)
}

func (a *App) applyIssuesTableTheme(table *tview.Table) {
	if table == nil {
		return
	}
	table.SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	table.SetSelectedStyle(selectionStyle(a.theme))
}

func (a *App) recolorNavigationTree() {
	if a.navigationTree == nil {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}
	a.applyNavigationNodeColors(root)
}

func (a *App) applyNavigationNodeColors(node *tview.TreeNode) {
	if node == nil {
		return
	}
	ref := node.GetReference()
	if ref == nil {
		node.SetColor(a.theme.Accent)
	} else if navNode, ok := ref.(*NavigationNode); ok {
		node.SetColor(a.navRowColor(navNode))
	}
	for _, child := range node.GetChildren() {
		a.applyNavigationNodeColors(child)
	}
}

// selectionStyle is the selected-row style shared by the tree and every issue
// table. tview's default inverse-video selection paints text in the primitive
// background color, which is unreadable for themes with a transparent
// background. Every list that marks a live selection composes with this, so
// changing how a selected row paints is one edit.
func selectionStyle(theme Theme) tcell.Style {
	return tcell.StyleDefault.
		Foreground(theme.SelectionText).
		Background(theme.SelectionBg).
		Bold(true)
}

// listSelectionStyle is the stronger accent selection, for a list that shares a
// modal with other controls: the form's inline multi-select and the prompt
// templates list, where the bar has to read as "the keyboard is in here" rather
// than "this row is current". A list that is the whole of its modal takes
// selectionStyle instead, the same bar the panes use.
func (a *App) listSelectionStyle() tcell.Style {
	return tcell.StyleDefault.
		Foreground(a.theme.InverseTextColor()).
		Background(a.theme.Accent).
		Bold(true)
}
