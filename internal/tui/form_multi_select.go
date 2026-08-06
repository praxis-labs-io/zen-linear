package tui

import (
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FormMultiSelect is FormModal's inline multi-select field: a framed list of
// toggle rows, no overlay. The form owns its keys, so it takes Space, t and
// Enter through FormModal.HandleKey.
type FormMultiSelect struct {
	list        *tview.List
	items       []MultiSelectItem
	selected    map[string]bool
	placeholder string
}

// AddMultiSelect appends a multi-select field under a caps label. rows is the
// preferred height; the row shrinks first when the screen is short.
func (fm *FormModal) AddMultiSelect(label string, rows int) *FormMultiSelect {
	theme := fm.app.theme

	ms := &FormMultiSelect{
		selected:    make(map[string]bool),
		placeholder: "No options",
	}
	ms.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.Foreground(theme.Foreground).Background(theme.ModalBackground())).
		SetSelectedStyle(fm.app.listSelectionStyle()).
		SetHighlightFullLine(true)
	// The cursor bar is a focus cue, not a selection: unfocused it reads as a
	// ticked row and competes with the field that does have focus.
	ms.list.SetSelectedFocusOnly(true)
	ms.list.SetBackgroundColor(theme.ModalBackground())
	ms.refresh()

	fm.addFramedRow(label, ms.list, rows, true)
	fm.multiSelects[ms.list] = ms
	return ms
}

// SetPlaceholder sets the single dim row shown when there are no options,
// so a loading fetch and an empty result read differently.
func (ms *FormMultiSelect) SetPlaceholder(text string) {
	ms.placeholder = text
	if len(ms.items) == 0 {
		ms.refresh()
	}
}

// SetItems replaces the options and the selection, keeping the highlight at
// the top.
func (ms *FormMultiSelect) SetItems(items []MultiSelectItem, selectedIDs []string) {
	ms.items = items
	ms.selected = make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		ms.selected[id] = true
	}
	ms.refresh()
}

// SelectedIDs returns the checked option ids, sorted so a caller can compare
// them against a previous selection.
func (ms *FormMultiSelect) SelectedIDs() []string {
	ids := make([]string, 0, len(ms.selected))
	for id := range ms.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// toggle flips the highlighted option.
func (ms *FormMultiSelect) toggle() {
	idx := ms.list.GetCurrentItem()
	if idx < 0 || idx >= len(ms.items) {
		return
	}
	id := ms.items[idx].ID
	if ms.selected[id] {
		delete(ms.selected, id)
	} else {
		ms.selected[id] = true
	}
	ms.refresh()
	ms.list.SetCurrentItem(idx)
}

// refresh rebuilds the rows. Parentheses mark the boxes because tview reads
// square brackets as a color tag.
func (ms *FormMultiSelect) refresh() {
	current := ms.list.GetCurrentItem()
	ms.list.Clear()

	if len(ms.items) == 0 {
		ms.list.AddItem(ms.placeholder, "", 0, nil)
		return
	}
	for _, item := range ms.items {
		prefix := "( ) "
		if ms.selected[item.ID] {
			prefix = "(x) "
		}
		ms.list.AddItem(prefix+item.Label, "", 0, nil)
	}
	if current < 0 || current >= len(ms.items) {
		current = 0
	}
	ms.list.SetCurrentItem(current)
}
