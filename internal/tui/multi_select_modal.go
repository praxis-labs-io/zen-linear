package tui

import (
	"sort"

	"github.com/gdamore/tcell/v2"
)

const (
	multiSelectMaxWidth = 60
	// multiSelectMaxVisibleRows caps the panel's height; a longer list scrolls.
	multiSelectMaxVisibleRows = 12
)

// MultiSelectItem represents an option in a multi-select modal.
type MultiSelectItem struct {
	ID    string
	Label string
}

// multiSelectGlyph is the box a toggle row leads with. Square brackets are out
// as the box, tview reads them as a color tag.
func multiSelectGlyph(on bool) string {
	if on {
		return "◼"
	}
	return "◻"
}

// multiSelectRow is one toggle row: a filled block in the color a finished
// action is said in, or a hollow one sitting back in the muted text.
func (a *App) multiSelectRow(label string, on bool) string {
	tag := a.themeTags.SecondaryText
	if on {
		tag = a.themeTags.Success
	}
	return tag + multiSelectGlyph(on) + "[-] " + label
}

// MultiSelectModal manages a reusable multi-select picker. Editing an issue's
// labels is this modal with a context line, not a second copy of it.
type MultiSelectModal struct {
	*listModal
	items    []MultiSelectItem
	selected map[string]bool
	onSave   func([]string)
}

// NewMultiSelectModal creates a new multi-select modal.
func NewMultiSelectModal(app *App) *MultiSelectModal {
	return &MultiSelectModal{
		listModal: newListModal(app, "multi_select",
			"↑↓ move   space toggle   ↵ apply   esc close",
			multiSelectMaxWidth, multiSelectMaxVisibleRows),
		selected: make(map[string]bool),
	}
}

// Show displays the multi-select modal.
func (mm *MultiSelectModal) Show(title string, items []MultiSelectItem, selectedIDs []string, onSave func([]string)) {
	mm.ShowWithContext(title, "", items, selectedIDs, onSave)
}

// ShowWithContext also pins an issue context line above the list, for the
// modals that toggle something about one issue.
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

// fillList rewrites the options, or the placeholder standing in for none.
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

// toggleCurrentItem flips the highlighted option and rewrites its row alone.
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

// HandleKey handles keyboard input for the multi-select modal.
func (mm *MultiSelectModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		mm.Hide()
		return nil
	case tcell.KeyEnter:
		// Nothing to apply when there was nothing to pick from. An empty
		// selection over real options is a choice; over none it is not.
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
