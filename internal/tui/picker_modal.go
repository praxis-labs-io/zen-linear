package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	pickerMaxWidth       = 50
	pickerMaxVisibleRows = 12
)

type PickerItem struct {
	ID    string
	Label string
	// Name is the value without the row's decoration, empty when the two are the same.
	Name string
}

func (i PickerItem) name() string {
	if i.Name != "" {
		return i.Name
	}
	return i.Label
}

type PickerModal struct {
	*listModal
	items    []PickerItem
	onSelect func(item PickerItem)
}

func NewPickerModal(app *App) *PickerModal {
	pm := &PickerModal{
		listModal: newListModal(app, "picker", "↑↓ move   ↵ select   esc close", pickerMaxWidth, pickerMaxVisibleRows),
	}
	pm.list.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action != tview.MouseLeftClick {
			return action, event
		}
		x, y := event.Position()
		if !pm.list.InRect(x, y) {
			return action, event
		}
		pm.chooseAt(x, y)
		return action, nil
	})
	return pm
}

func (pm *PickerModal) chooseAt(x, y int) {
	left, top, width, height := pm.list.GetInnerRect()
	if x < left || x >= left+width || y < top || y >= top+height {
		return
	}
	offset, _ := pm.list.GetOffset()
	pm.choose(y - top + offset)
}

func (pm *PickerModal) Show(title string, items []PickerItem, onSelect func(item PickerItem)) {
	pm.ShowWithContext(title, "", items, onSelect)
}

func (pm *PickerModal) ShowWithContext(title, contextLine string, items []PickerItem, onSelect func(item PickerItem)) {
	pm.items = items
	pm.onSelect = onSelect

	pm.fillList()
	pm.open(title, contextLine)
}

func (pm *PickerModal) fillList() {
	if len(pm.items) == 0 {
		pm.showPlaceholder("No options")
		return
	}

	pm.beginRows(len(pm.items))
	for index, item := range pm.items {
		var shortcut rune
		if index < 9 {
			shortcut = rune('1' + index)
		}
		pm.list.AddItem(item.Label, "", shortcut, nil)
	}
	pm.list.SetCurrentItem(0)
}

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
