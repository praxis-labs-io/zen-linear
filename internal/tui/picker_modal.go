package tui

import (
	"github.com/gdamore/tcell/v2"
)

const (
	pickerMaxWidth = 50
	// pickerMaxVisibleRows caps the panel's height; a longer list scrolls.
	pickerMaxVisibleRows = 12
)

// PickerItem represents an item in a picker.
type PickerItem struct {
	ID    string
	Label string
}

// PickerModal manages a picker overlay for selecting from a list of items.
type PickerModal struct {
	*listModal
	items    []PickerItem
	onSelect func(item PickerItem)
}

// NewPickerModal creates a new picker modal.
func NewPickerModal(app *App) *PickerModal {
	return &PickerModal{
		listModal: newListModal(app, "picker", "↑↓ move   ↵ select   esc close", pickerMaxWidth, pickerMaxVisibleRows),
	}
}

// Show displays the picker modal with the given title and items.
func (pm *PickerModal) Show(title string, items []PickerItem, onSelect func(item PickerItem)) {
	pm.ShowWithContext(title, "", items, onSelect)
}

// ShowWithContext also pins an issue context line above the list.
func (pm *PickerModal) ShowWithContext(title, contextLine string, items []PickerItem, onSelect func(item PickerItem)) {
	pm.items = items
	pm.onSelect = onSelect

	pm.fillList()
	pm.open(title, contextLine)
}

// fillList rewrites the options, or the placeholder standing in for none.
func (pm *PickerModal) fillList() {
	if len(pm.items) == 0 {
		pm.showPlaceholder("No options")
		return
	}

	pm.beginRows(len(pm.items))
	for index, item := range pm.items {
		// Number shortcuts select the first nine entries directly.
		var shortcut rune
		if index < 9 {
			shortcut = rune('1' + index)
		}
		// The selected func only ever fires on a click: HandleKey answers Enter
		// and the number keys before the list sees either.
		pm.list.AddItem(item.Label, "", shortcut, func() { pm.choose(index) })
	}
	pm.list.SetCurrentItem(0)
}

// choose closes the picker and reports the item at the given index.
func (pm *PickerModal) choose(index int) {
	if index < 0 || index >= len(pm.items) {
		return
	}
	item := pm.items[index]
	pm.Hide()
	if pm.onSelect != nil {
		pm.onSelect(item)
	}
}

// HandleKey handles keyboard input for the picker.
func (pm *PickerModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		pm.Hide()
		return nil
	case tcell.KeyEnter:
		pm.choose(pm.list.GetCurrentItem())
		return nil
	case tcell.KeyUp:
		pm.move(-1)
		return nil
	case tcell.KeyDown:
		pm.move(1)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'j':
			pm.move(1)
			return nil
		case 'k':
			pm.move(-1)
			return nil
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			pm.choose(int(event.Rune() - '1'))
			return nil
		}
	}
	return event
}
