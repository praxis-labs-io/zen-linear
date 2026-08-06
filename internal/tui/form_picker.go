package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// formPickerMenuRows caps how tall an open menu grows. Past it the menu
// scrolls, so a team with forty projects gets a menu the size of a short list
// instead of one taller than the form.
const formPickerMenuRows = 8

// FormPicker is FormModal's dropdown: a framed value the user reads, and a
// menu that drops under it on Enter. tview's own DropDown sizes its menu to
// the option count and only clamps it at the terminal edge, with no way in to
// cap it, so the form owns the menu instead.
type FormPicker struct {
	fm       *FormModal
	view     *tview.TextView
	options  []string
	selected int
	onChange func(text string, index int)
}

// AddPicker appends a picker. Consecutive AddPicker calls share one two-row
// unit: caps labels on the first row, the fields beneath, equal widths.
func (fm *FormModal) AddPicker(label string, options []string, selected int, onChange func(text string, index int)) *FormPicker {
	view := tview.NewTextView()
	view.SetTextColor(fm.app.theme.Foreground)
	view.SetBackgroundColor(fm.app.theme.ModalBackground())

	picker := &FormPicker{
		fm:       fm,
		view:     view,
		options:  append([]string(nil), options...),
		selected: -1,
		onChange: onChange,
	}
	rowIdx := fm.packField(label, view)
	fm.pickers[view] = picker
	fm.registerFocusable(view, rowIdx)
	picker.SetCurrentOption(selected)
	return picker
}

// SetPickerOptions replaces a picker's options and change handler, clearing
// the selection the way tview's DropDown does. Callers set the new selection
// afterwards.
func (fm *FormModal) SetPickerOptions(picker *FormPicker, options []string, onChange func(text string, index int)) {
	if picker.isOpen() {
		picker.closeMenu()
	}
	picker.options = append([]string(nil), options...)
	picker.onChange = onChange
	picker.selected = -1
	picker.view.SetText("")
}

// SetCurrentOption selects an option and runs the change handler, so a caller
// that sets the value and a user who picks one land in the same place. An
// index outside the options clears the selection.
func (picker *FormPicker) SetCurrentOption(index int) {
	if index < 0 || index >= len(picker.options) {
		picker.selected = -1
		picker.view.SetText("")
		if picker.onChange != nil {
			picker.onChange("", -1)
		}
		return
	}
	picker.selected = index
	picker.view.SetText(picker.options[index])
	if picker.onChange != nil {
		picker.onChange(picker.options[index], index)
	}
}

// GetCurrentOption returns the selected index and its text, or -1 and an
// empty string when nothing is selected.
func (picker *FormPicker) GetCurrentOption() (int, string) {
	if picker.selected < 0 || picker.selected >= len(picker.options) {
		return -1, ""
	}
	return picker.selected, picker.options[picker.selected]
}

// IsOpen reports whether this picker's menu is showing.
func (picker *FormPicker) IsOpen() bool { return picker.isOpen() }

func (picker *FormPicker) isOpen() bool { return picker.fm.openPicker == picker }

// openMenu drops the menu under the field, highlighting the current value.
func (picker *FormPicker) openMenu() {
	if len(picker.options) == 0 {
		return
	}
	picker.fm.openPicker = picker

	menu := picker.fm.menu
	menu.Clear()
	for _, option := range picker.options {
		menu.AddItem(option, "", 0, nil)
	}
	current := picker.selected
	if current < 0 {
		current = 0
	}
	menu.SetCurrentItem(current)
}

func (picker *FormPicker) closeMenu() {
	if picker.isOpen() {
		picker.fm.openPicker = nil
	}
}

// commitMenu takes the highlighted row and closes the menu.
func (picker *FormPicker) commitMenu() {
	index := picker.fm.menu.GetCurrentItem()
	picker.closeMenu()
	picker.SetCurrentOption(index)
}

// focusedPicker returns the picker holding focus, if any.
func (fm *FormModal) focusedPicker() *FormPicker {
	view, ok := fm.focusedPrimitive().(*tview.TextView)
	if !ok {
		return nil
	}
	return fm.pickers[view]
}

// moveMenu walks the open menu without wrapping, so holding a key does not
// loop past the ends.
func (fm *FormModal) moveMenu(delta int) {
	index := fm.menu.GetCurrentItem() + delta
	if index < 0 {
		index = 0
	}
	if last := fm.menu.GetItemCount() - 1; index > last {
		index = last
	}
	fm.menu.SetCurrentItem(index)
}

// handleMenuKey owns every key while a menu is open.
func (fm *FormModal) handleMenuKey(event *tcell.EventKey) *tcell.EventKey {
	picker := fm.openPicker
	switch event.Key() {
	case tcell.KeyEscape:
		picker.closeMenu()
	case tcell.KeyEnter:
		picker.commitMenu()
	case tcell.KeyUp:
		fm.moveMenu(-1)
	case tcell.KeyDown:
		fm.moveMenu(1)
	case tcell.KeyTab:
		picker.closeMenu()
		fm.focusStep(1)
	case tcell.KeyBacktab:
		picker.closeMenu()
		fm.focusStep(-1)
	case tcell.KeyRune:
		switch event.Rune() {
		case 'k':
			fm.moveMenu(-1)
		case 'j':
			fm.moveMenu(1)
		}
	}
	return nil
}

// drawOpenMenu paints the open menu over the form. It is drawn after the rows
// because a Flex has no z-order: a menu that was a row would push the fields
// below it down the form.
func (fm *FormModal) drawOpenMenu(screen tcell.Screen) {
	picker := fm.openPicker
	if picker == nil {
		return
	}
	frame, ok := fm.frameOf[picker.view]
	if !ok {
		return
	}

	fieldX, fieldY, fieldWidth, fieldHeight := frame.GetRect()
	_, screenH := screen.Size()
	x, y, width, height, fits := pickerMenuRect(fieldX, fieldY, fieldWidth, fieldHeight, len(picker.options), screenH)
	if !fits {
		return
	}

	fm.menu.SetRect(x, y, width, height)
	fm.menu.Draw(screen)
}

// pickerMenuRect places the menu against its field: same left edge and width
// so the borders line up, on the row right after the field, which is as close
// as a character grid gets without the two borders merging into one line. It
// drops up when the screen ends first. The returned rect includes the border.
func pickerMenuRect(fieldX, fieldY, fieldWidth, fieldHeight, optionCount, screenH int) (x, y, width, height int, fits bool) {
	rows := optionCount
	if rows > formPickerMenuRows {
		rows = formPickerMenuRows
	}
	height = rows + 2 // border
	if fieldWidth < 3 || rows < 1 {
		return 0, 0, 0, 0, false
	}

	y = fieldY + fieldHeight
	if y+height > screenH {
		if above := fieldY - height; above >= 0 {
			y = above
		} else if height = screenH - y; height < 3 {
			return 0, 0, 0, 0, false
		}
	}
	return fieldX, y, fieldWidth, height, true
}
