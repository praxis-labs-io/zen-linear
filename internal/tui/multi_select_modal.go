package tui

import (
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MultiSelectItem represents an option in a multi-select modal.
type MultiSelectItem struct {
	ID    string
	Label string
}

// MultiSelectModal manages a reusable multi-select picker.
type MultiSelectModal struct {
	app       *App
	modal     *tview.Flex
	list      *tview.List
	titleView *tview.TextView
	items     []MultiSelectItem
	selected  map[string]bool
	onSave    func([]string)
}

// NewMultiSelectModal creates a new multi-select modal.
func NewMultiSelectModal(app *App) *MultiSelectModal {
	mm := &MultiSelectModal{
		app:      app,
		selected: make(map[string]bool),
	}

	mm.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextColor(app.theme.Foreground).
		SetSelectedBackgroundColor(app.theme.Accent).
		SetSelectedTextColor(app.theme.SelectionText).
		SetHighlightFullLine(true)
	mm.list.SetBackgroundColor(app.theme.HeaderBg)

	mm.titleView = tview.NewTextView()
	mm.titleView.SetTextColor(app.theme.Accent)
	mm.titleView.SetBackgroundColor(app.theme.HeaderBg)

	helpView := tview.NewTextView()
	helpView.SetText("Space: toggle | Enter: apply | Esc: cancel")
	helpView.SetTextColor(app.theme.SecondaryText)
	helpView.SetBackgroundColor(app.theme.HeaderBg)
	helpView.SetTextAlign(tview.AlignCenter)

	content := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mm.titleView, 1, 0, false).
		AddItem(mm.list, 0, 1, true).
		AddItem(helpView, 1, 0, false)
	content.Box = tview.NewBox().SetBackgroundColor(app.theme.HeaderBg)
	content.SetBackgroundColor(app.theme.HeaderBg).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	content.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	mm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(content, 20, 0, true).
			AddItem(nil, 0, 1, false), 60, 0, true).
		AddItem(nil, 0, 1, false)
	mm.modal.SetBackgroundColor(app.theme.Background)

	return mm
}

// Show displays the multi-select modal.
func (mm *MultiSelectModal) Show(title string, items []MultiSelectItem, selectedIDs []string, onSave func([]string)) {
	mm.items = items
	mm.onSave = onSave
	mm.selected = make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		mm.selected[id] = true
	}
	mm.titleView.SetText(title)
	mm.refreshList()
	mm.app.pages.AddPage("multi_select", mm.modal, true, true)
	mm.app.pages.SendToFront("multi_select")
	mm.app.app.SetFocus(mm.list)
}

func (mm *MultiSelectModal) refreshList() {
	currentIdx := mm.list.GetCurrentItem()
	mm.list.Clear()
	if len(mm.items) == 0 {
		mm.list.AddItem("No options available", "", 0, nil)
		return
	}
	for idx, item := range mm.items {
		focusPrefix := "  "
		if idx == currentIdx {
			focusPrefix = "> "
		}
		prefix := "( ) "
		if mm.selected[item.ID] {
			prefix = "(x) "
		}
		mm.list.AddItem(focusPrefix+prefix+item.Label, "", 0, nil)
	}
	if currentIdx < 0 || currentIdx >= len(mm.items) {
		currentIdx = 0
	}
	mm.list.SetCurrentItem(currentIdx)
}

func (mm *MultiSelectModal) toggleCurrentItem() {
	idx := mm.list.GetCurrentItem()
	if idx < 0 || idx >= len(mm.items) {
		return
	}
	id := mm.items[idx].ID
	if mm.selected[id] {
		delete(mm.selected, id)
	} else {
		mm.selected[id] = true
	}
	mm.refreshList()
	mm.list.SetCurrentItem(idx)
}

func (mm *MultiSelectModal) selectedIDs() []string {
	ids := make([]string, 0, len(mm.selected))
	for id := range mm.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Hide hides the multi-select modal.
func (mm *MultiSelectModal) Hide() {
	mm.app.pages.RemovePage("multi_select")
	mm.app.updateFocus()
}

// HandleKey handles keyboard input for the multi-select modal.
func (mm *MultiSelectModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		mm.Hide()
		return nil
	case tcell.KeyEnter:
		ids := mm.selectedIDs()
		mm.Hide()
		if mm.onSave != nil {
			mm.onSave(ids)
		}
		return nil
	case tcell.KeyUp:
		mm.moveCurrentItem(-1)
		return nil
	case tcell.KeyDown:
		mm.moveCurrentItem(1)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case ' ', 't':
			mm.toggleCurrentItem()
			return nil
		case 'j':
			mm.moveCurrentItem(1)
			return nil
		case 'k':
			mm.moveCurrentItem(-1)
			return nil
		}
	}
	return event
}

func (mm *MultiSelectModal) moveCurrentItem(delta int) {
	if len(mm.items) == 0 {
		return
	}
	idx := mm.list.GetCurrentItem() + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(mm.items) {
		idx = len(mm.items) - 1
	}
	mm.list.SetCurrentItem(idx)
	mm.refreshList()
}
