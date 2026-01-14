package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// EditLabelsModal manages a multi-select modal for editing issue labels.
type EditLabelsModal struct {
	app       *App
	modal     *tview.Flex
	list      *tview.List
	titleView *tview.TextView
	helpView  *tview.TextView

	// State
	issueID         string
	availableLabels []linearapi.IssueLabel
	selectedIDs     map[string]bool // Track which label IDs are selected
	onSave          func(issueID string, labelIDs []string)
}

// NewEditLabelsModal creates a new edit labels modal.
func NewEditLabelsModal(app *App) *EditLabelsModal {
	elm := &EditLabelsModal{
		app:         app,
		selectedIDs: make(map[string]bool),
	}

	// Create list for labels
	elm.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(LinearTheme.Accent).
		SetSelectedTextColor(tcell.ColorWhite).
		SetHighlightFullLine(true)
	elm.list.SetBackgroundColor(LinearTheme.HeaderBg)

	// Create title
	elm.titleView = tview.NewTextView()
	elm.titleView.SetText("Edit Labels")
	elm.titleView.SetTextColor(tcell.ColorYellow)
	elm.titleView.SetBackgroundColor(LinearTheme.HeaderBg)

	// Create help text
	elm.helpView = tview.NewTextView()
	elm.helpView.SetText("Space: toggle | Enter: save | Esc: cancel")
	elm.helpView.SetTextColor(tcell.ColorGray)
	elm.helpView.SetBackgroundColor(LinearTheme.HeaderBg)
	elm.helpView.SetTextAlign(tview.AlignCenter)

	// Build modal content
	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(elm.titleView, 1, 0, false).
		AddItem(elm.list, 0, 1, true).
		AddItem(elm.helpView, 1, 0, false)
	modalContent.SetBackgroundColor(LinearTheme.HeaderBg).
		SetBorder(true).
		SetBorderColor(LinearTheme.Accent).
		SetTitle(" Edit Labels ").
		SetTitleColor(tcell.ColorWhite)

	// Center the modal on screen
	elm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, 20, 0, true).
			AddItem(nil, 0, 1, false), 60, 0, true).
		AddItem(nil, 0, 1, false)
	elm.modal.SetBackgroundColor(LinearTheme.Background)

	return elm
}

// Show displays the edit labels modal with available labels and current selections.
func (elm *EditLabelsModal) Show(issueID string, currentLabelIDs []string, availableLabels []linearapi.IssueLabel, onSave func(issueID string, labelIDs []string)) {
	elm.issueID = issueID
	elm.availableLabels = availableLabels
	elm.onSave = onSave

	// Initialize selected IDs from current labels
	elm.selectedIDs = make(map[string]bool)
	for _, id := range currentLabelIDs {
		elm.selectedIDs[id] = true
	}

	// Populate list
	elm.refreshList()

	// Show modal
	elm.app.pages.AddPage("edit_labels", elm.modal, true, true)
	elm.app.pages.SendToFront("edit_labels")
	elm.app.app.SetFocus(elm.list)
}

// refreshList rebuilds the list with current selection state.
func (elm *EditLabelsModal) refreshList() {
	elm.list.Clear()

	for _, label := range elm.availableLabels {
		// Build display text with selection indicator
		// Using parentheses to avoid tview interpreting [] as color tags
		prefix := "( ) "
		if elm.selectedIDs[label.ID] {
			prefix = "(•) "
		}
		displayText := prefix + label.Name

		elm.list.AddItem(displayText, "", 0, nil)
	}

	if len(elm.availableLabels) > 0 {
		elm.list.SetCurrentItem(0)
	}
}

// toggleCurrentItem toggles the selection state of the currently highlighted item.
func (elm *EditLabelsModal) toggleCurrentItem() {
	idx := elm.list.GetCurrentItem()
	if idx < 0 || idx >= len(elm.availableLabels) {
		return
	}

	labelID := elm.availableLabels[idx].ID
	if elm.selectedIDs[labelID] {
		delete(elm.selectedIDs, labelID)
	} else {
		elm.selectedIDs[labelID] = true
	}

	// Refresh list to update checkbox display
	currentIdx := elm.list.GetCurrentItem()
	elm.refreshList()
	elm.list.SetCurrentItem(currentIdx)
}

// getSelectedLabelIDs returns a slice of currently selected label IDs.
func (elm *EditLabelsModal) getSelectedLabelIDs() []string {
	ids := make([]string, 0, len(elm.selectedIDs))
	for id := range elm.selectedIDs {
		ids = append(ids, id)
	}
	return ids
}

// Hide hides the edit labels modal.
func (elm *EditLabelsModal) Hide() {
	elm.app.pages.RemovePage("edit_labels")
	elm.app.updateFocus()
}

// HandleKey handles keyboard input for the edit labels modal.
func (elm *EditLabelsModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		elm.Hide()
		return nil
	case tcell.KeyEnter:
		// Save and close
		selectedIDs := elm.getSelectedLabelIDs()
		elm.Hide()
		if elm.onSave != nil {
			elm.onSave(elm.issueID, selectedIDs)
		}
		return nil
	case tcell.KeyUp:
		idx := elm.list.GetCurrentItem()
		if idx > 0 {
			elm.list.SetCurrentItem(idx - 1)
		}
		return nil
	case tcell.KeyDown:
		idx := elm.list.GetCurrentItem()
		if idx < elm.list.GetItemCount()-1 {
			elm.list.SetCurrentItem(idx + 1)
		}
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case ' ':
			elm.toggleCurrentItem()
			return nil
		case 'j':
			idx := elm.list.GetCurrentItem()
			if idx < elm.list.GetItemCount()-1 {
				elm.list.SetCurrentItem(idx + 1)
			}
			return nil
		case 'k':
			idx := elm.list.GetCurrentItem()
			if idx > 0 {
				elm.list.SetCurrentItem(idx - 1)
			}
			return nil
		}
	}
	return event
}

// GetModal returns the modal flex for adding to pages.
func (elm *EditLabelsModal) GetModal() *tview.Flex {
	return elm.modal
}
