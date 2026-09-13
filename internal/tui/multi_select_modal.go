package tui

import (
	"sort"

	"github.com/gdamore/tcell/v2"
)

const (
	multiSelectMaxWidth       = 60
	multiSelectMaxVisibleRows = 12
)

type MultiSelectItem struct {
	ID    string
	Label string
}

// Not square brackets: tview reads those as a color tag.
func multiSelectGlyph(on bool) string {
	if on {
		return "◼"
	}
	return "◻"
}

func (a *App) multiSelectRow(label string, on bool) string {
	tag := a.themeTags.SecondaryText
	if on {
		tag = a.themeTags.Success
	}
	return tag + multiSelectGlyph(on) + "[-] " + label
}

type MultiSelectModal struct {
	*listModal
	items    []MultiSelectItem
	selected map[string]bool
	onSave   func([]string)
}

func NewMultiSelectModal(app *App) *MultiSelectModal {
	return &MultiSelectModal{
		listModal: newListModal(app, "multi_select",
			"↑↓ move   space toggle   ↵ apply   esc close",
			multiSelectMaxWidth, multiSelectMaxVisibleRows),
		selected: make(map[string]bool),
	}
}

func (mm *MultiSelectModal) Show(title string, items []MultiSelectItem, selectedIDs []string, onSave func([]string)) {
	mm.ShowWithContext(title, "", items, selectedIDs, onSave)
}

func (mm *MultiSelectModal) ShowWithContext(title, contextLine string, items []MultiSelectItem, selectedIDs []string, onSave func([]string)) {
	mm.items = items
	mm.onSave = onSave
	mm.selected = make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		mm.selected[id] = true
	}

	mm.fillList()
	mm.open(title, contextLine)
}

func (mm *MultiSelectModal) fillList() {
	if len(mm.items) == 0 {
		mm.showPlaceholder("No options")
		return
	}

	mm.beginRows(len(mm.items))
	for _, item := range mm.items {
		mm.list.AddItem(mm.app.multiSelectRow(item.Label, mm.selected[item.ID]), "", 0, nil)
	}
	mm.list.SetCurrentItem(0)
}

func (mm *MultiSelectModal) toggleCurrentItem() {
	index := mm.list.GetCurrentItem()
	if index < 0 || index >= len(mm.items) {
		return
	}
	item := mm.items[index]
	if mm.selected[item.ID] {
		delete(mm.selected, item.ID)
	} else {
		mm.selected[item.ID] = true
	}
	mm.list.SetItemText(index, mm.app.multiSelectRow(item.Label, mm.selected[item.ID]), "")
}

func (mm *MultiSelectModal) selectedIDs() []string {
	ids := make([]string, 0, len(mm.selected))
	for id := range mm.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (mm *MultiSelectModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		mm.Hide()
		return nil
	case tcell.KeyEnter:
		if len(mm.items) == 0 {
			mm.Hide()
			return nil
		}
		ids := mm.selectedIDs()
		mm.Hide()
		if mm.onSave != nil {
			mm.onSave(ids)
		}
		return nil
	case tcell.KeyUp:
		mm.move(-1)
		return nil
	case tcell.KeyDown:
		mm.move(1)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case ' ', 't':
			mm.toggleCurrentItem()
			return nil
		case 'j':
			mm.move(1)
			return nil
		case 'k':
			mm.move(-1)
			return nil
		}
	}
	return event
}
