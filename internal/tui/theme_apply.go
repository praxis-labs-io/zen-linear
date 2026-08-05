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
		a.navigationTree.SetBackgroundColor(a.theme.Background).
			SetBorderColor(a.theme.Border).
			SetTitleColor(a.theme.Foreground)
		a.recolorNavigationTree()
	}

	if a.myIssuesTable != nil {
		a.applyIssuesTableTheme(a.myIssuesTable)
		renderIssuesTableModel(a.myIssuesTable, a.myIssueRows, a.myIDToIssue, a.selectedIssueID(IssuesSectionMy), a.theme, a.issueColumns())
	}
	if a.allIssuesTable != nil {
		a.applyIssuesTableTheme(a.allIssuesTable)
		renderIssuesTableModel(a.allIssuesTable, a.allIssueRows, a.allIDToIssue, a.selectedIssueID(IssuesSectionAll), a.theme, a.issueColumns())
	}
	if a.searchPanel != nil {
		// Rebuild the panel so the input picks up the new InputBg (tview
		// bakes it at construction), then restyle and re-render the results.
		a.applyIssuesTableTheme(a.searchResultsTable)
		a.buildSearchPanel()
		renderIssuesTableModel(a.searchResultsTable, a.searchIssueRows, a.searchIDToIssue, a.selectedIssueID(IssuesSectionSearch), a.theme, a.issueColumns())
		a.updateIssuesColumnLayout()
	}

	if a.detailsDescriptionView != nil {
		a.detailsDescriptionView.SetTitleColor(a.theme.Foreground).
			SetBorderColor(a.theme.Border).
			SetBackgroundColor(a.theme.Background)
	}
	if a.detailsCommentsView != nil {
		a.detailsCommentsView.SetTitleColor(a.theme.Foreground).
			SetBorderColor(a.theme.Border).
			SetBackgroundColor(a.theme.Background)
	}

	if a.statusBar != nil {
		a.statusBar.SetBackgroundColor(a.theme.HeaderBg)
	}
}

func (a *App) applyDensityToComponents() {
	if a.detailsDescriptionView != nil {
		padding := a.density.DetailsPadding
		a.detailsDescriptionView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	}
	if a.detailsCommentsView != nil {
		padding := a.density.DetailsPadding
		a.detailsCommentsView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	}
	if a.statusBar != nil {
		padding := a.density.StatusBarPadding
		a.statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	}
	if a.agentOutputModal != nil {
		a.agentOutputModal.ApplyDensity(a.density)
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
	a.createIssueModal = NewCreateIssueModal(a)
	a.createCommentModal = NewCreateCommentModal(a)
	a.editTitleModal = NewEditTitleModal(a)
	a.editDescriptionModal = NewEditDescriptionModal(a)
	a.editLabelsModal = NewEditLabelsModal(a)
	a.textInputModal = NewTextInputModal(a)
	a.multiSelectModal = NewMultiSelectModal(a)
	a.settingsModal = NewSettingsModal(a)
	a.promptTemplatesModal = NewAgentPromptTemplatesModal(a)
	a.agentPromptModal = NewAgentPromptModal(a)
	if a.pages == nil || !a.pages.HasPage("agent_output") {
		a.agentOutputModal = NewAgentOutputModal(a)
	} else {
		a.agentOutputModal.ApplyTheme(a.theme)
		a.agentOutputModal.ApplyDensity(a.density)
	}
	a.confirmationModal = NewConfirmationModal(a)
}

func (a *App) applyIssuesTableTheme(table *tview.Table) {
	if table == nil {
		return
	}
	table.SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	table.SetSelectedStyle(tcell.StyleDefault.
		Foreground(a.theme.SelectionText).
		Background(a.theme.SelectionBg).
		Bold(true))
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
		if navNode.IsProject || navNode.IsStatus {
			node.SetColor(a.theme.SecondaryText)
		} else {
			node.SetColor(a.theme.Foreground)
		}
	}
	node.SetSelectedTextStyle(a.selectionStyle())
	for _, child := range node.GetChildren() {
		a.applyNavigationNodeColors(child)
	}
}

// selectionStyle is the selected-row style shared by the tree and tables.
// tview's default inverse-video selection paints text in the primitive
// background color, which is unreadable for themes with a transparent
// background.
func (a *App) selectionStyle() tcell.Style {
	return tcell.StyleDefault.
		Foreground(a.theme.SelectionText).
		Background(a.theme.SelectionBg).
		Bold(true)
}

// listSelectionStyle is the stronger accent selection used by modal lists
// (command palette, pickers), where the selected row is the primary object on
// screen and must stand apart from input fields and panel fills.
func (a *App) listSelectionStyle() tcell.Style {
	return tcell.StyleDefault.
		Foreground(a.theme.InverseTextColor()).
		Background(a.theme.Accent).
		Bold(true)
}

// applySelectionStyleToTree sets the shared selection style on a node subtree
// without touching node colors.
func (a *App) applySelectionStyleToTree(node *tview.TreeNode) {
	if node == nil {
		return
	}
	node.SetSelectedTextStyle(a.selectionStyle())
	for _, child := range node.GetChildren() {
		a.applySelectionStyleToTree(child)
	}
}
