package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// PickerItem represents an item in a picker.
type PickerItem struct {
	ID    string
	Label string
}

// PickerModal manages a picker overlay for selecting from a list of items.
type PickerModal struct {
	app       *App
	modal     *tview.Flex
	list      *tview.List
	titleView *tview.TextView
	items     []PickerItem
	onSelect  func(item PickerItem)
}

// NewPickerModal creates a new picker modal.
func NewPickerModal(app *App) *PickerModal {
	pm := &PickerModal{
		app: app,
	}

	// Create list
	pm.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(LinearTheme.Accent).
		SetSelectedTextColor(tcell.ColorWhite).
		SetHighlightFullLine(true)
	pm.list.SetBackgroundColor(LinearTheme.HeaderBg)

	// Create title
	pm.titleView = tview.NewTextView()
	pm.titleView.SetTextColor(tcell.ColorYellow)
	pm.titleView.SetBackgroundColor(LinearTheme.HeaderBg)

	// Create help text
	helpText := tview.NewTextView()
	helpText.SetText("↑↓/j/k: navigate | Enter: select | Esc: cancel")
	helpText.SetTextColor(tcell.ColorGray)
	helpText.SetBackgroundColor(LinearTheme.HeaderBg)

	// Build modal content
	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pm.titleView, 1, 0, false).
		AddItem(pm.list, 0, 1, true).
		AddItem(helpText, 1, 0, false)
	modalContent.SetBackgroundColor(LinearTheme.HeaderBg).
		SetBorder(true).
		SetBorderColor(LinearTheme.Accent).
		SetTitleColor(tcell.ColorWhite)

	// Center the modal on screen
	pm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, 15, 0, true).
			AddItem(nil, 0, 1, false), 50, 0, true).
		AddItem(nil, 0, 1, false)
	pm.modal.SetBackgroundColor(LinearTheme.Background)

	return pm
}

// Show displays the picker modal with the given title and items.
func (pm *PickerModal) Show(title string, items []PickerItem, onSelect func(item PickerItem)) {
	pm.items = items
	pm.onSelect = onSelect

	pm.titleView.SetText(title)
	pm.list.Clear()

	for _, item := range items {
		pm.list.AddItem(item.Label, "", 0, nil)
	}

	if len(items) > 0 {
		pm.list.SetCurrentItem(0)
	}

	pm.app.pages.AddPage("picker", pm.modal, true, true)
	pm.app.pages.SendToFront("picker")
	pm.app.app.SetFocus(pm.list)
}

// Hide hides the picker modal.
func (pm *PickerModal) Hide() {
	pm.app.pickerActive = false
	pm.app.pages.RemovePage("picker")

	// Check if create issue modal is visible and restore focus to it
	if pm.app.pages.HasPage("create_issue") {
		pm.app.pages.SendToFront("create_issue")
		if pm.app.createIssueModal != nil {
			pm.app.app.SetFocus(pm.app.createIssueModal.form)
		}
		return
	}

	// Check if edit title modal is visible and restore focus to it
	if pm.app.pages.HasPage("edit_title") {
		pm.app.pages.SendToFront("edit_title")
		if pm.app.editTitleModal != nil {
			pm.app.app.SetFocus(pm.app.editTitleModal.form)
		}
		return
	}

	pm.app.updateFocus()
}

// HandleKey handles keyboard input for the picker.
func (pm *PickerModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		pm.Hide()
		return nil
	case tcell.KeyEnter:
		idx := pm.list.GetCurrentItem()
		if idx >= 0 && idx < len(pm.items) {
			item := pm.items[idx]
			pm.Hide()
			if pm.onSelect != nil {
				pm.onSelect(item)
			}
		}
		return nil
	case tcell.KeyUp:
		idx := pm.list.GetCurrentItem()
		if idx > 0 {
			pm.list.SetCurrentItem(idx - 1)
		}
		return nil
	case tcell.KeyDown:
		idx := pm.list.GetCurrentItem()
		if idx < pm.list.GetItemCount()-1 {
			pm.list.SetCurrentItem(idx + 1)
		}
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'j':
			idx := pm.list.GetCurrentItem()
			if idx < pm.list.GetItemCount()-1 {
				pm.list.SetCurrentItem(idx + 1)
			}
			return nil
		case 'k':
			idx := pm.list.GetCurrentItem()
			if idx > 0 {
				pm.list.SetCurrentItem(idx - 1)
			}
			return nil
		}
	}
	return event
}

// GetModal returns the modal flex for adding to pages.
func (pm *PickerModal) GetModal() *tview.Flex {
	return pm.modal
}
