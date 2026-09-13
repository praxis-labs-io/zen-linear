package tui

import (
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type FormMultiSelect struct {
	app         *App
	list        *tview.List
	items       []MultiSelectItem
	selected    map[string]bool
	placeholder string
}

func (fm *FormModal) newMultiSelect() *FormMultiSelect {
	theme := fm.app.theme

	ms := &FormMultiSelect{
		app:         fm.app,
		selected:    make(map[string]bool),
		placeholder: "No options",
	}
	ms.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.Foreground(theme.Foreground).Background(theme.ModalBackground())).
		SetSelectedStyle(fm.app.listSelectionStyle()).
		SetHighlightFullLine(true)
	ms.list.SetSelectedFocusOnly(true)
	ms.list.SetBackgroundColor(theme.ModalBackground())
	ms.refresh()

	fm.multiSelects[ms.list] = ms
	return ms
}

func (ms *FormMultiSelect) SetPlaceholder(text string) {
	ms.placeholder = text
	if len(ms.items) == 0 {
		ms.refresh()
	}
}

func (ms *FormMultiSelect) SetItems(items []MultiSelectItem, selectedIDs []string) {
	ms.items = items
	ms.selected = make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		ms.selected[id] = true
	}
	ms.refresh()
}

// SelectedIDs returns the checked option ids, sorted.
func (ms *FormMultiSelect) SelectedIDs() []string {
	ids := make([]string, 0, len(ms.selected))
	for id := range ms.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

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

func (ms *FormMultiSelect) refresh() {
	current := ms.list.GetCurrentItem()
	ms.list.Clear()

	if len(ms.items) == 0 {
		ms.list.AddItem(ms.placeholder, "", 0, nil)
		return
	}
	for _, item := range ms.items {
		ms.list.AddItem(ms.app.multiSelectRow(item.Label, ms.selected[item.ID]), "", 0, nil)
	}
	if current < 0 || current >= len(ms.items) {
		current = 0
	}
	ms.list.SetCurrentItem(current)
}
