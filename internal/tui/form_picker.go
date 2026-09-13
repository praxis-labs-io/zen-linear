package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const formPickerMenuRows = 8

// FormPicker replaces tview.DropDown, whose menu grows to the option count with no way to cap it.
type FormPicker struct {
	fm       *FormModal
	view     *tview.TextView
	options  []string
	selected int
	onChange func(text string, index int)
}

// AddPicker appends a picker; consecutive calls share one row at equal widths.
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

// SetPickerOptions replaces the options and handler and clears the selection; set a new one afterwards.
func (fm *FormModal) SetPickerOptions(picker *FormPicker, options []string, onChange func(text string, index int)) {
	if picker.isOpen() {
		picker.closeMenu()
	}
	picker.options = append([]string(nil), options...)
	picker.onChange = onChange
	picker.selected = -1
	picker.view.SetText("")
}

// SetCurrentOption selects index and runs the change handler; an index out of range clears the selection.
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

// GetCurrentOption returns -1 and "" when nothing is selected.
func (picker *FormPicker) GetCurrentOption() (int, string) {
	if picker.selected < 0 || picker.selected >= len(picker.options) {
		return -1, ""
	}
	return picker.selected, picker.options[picker.selected]
}

func (picker *FormPicker) View() tview.Primitive { return picker.view }

func (picker *FormPicker) IsOpen() bool { return picker.isOpen() }

func (picker *FormPicker) isOpen() bool { return picker.fm.openPicker == picker }

func (picker *FormPicker) openMenu() {
	if len(picker.options) == 0 || picker.fm.isLocked(picker.view) {
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

func (picker *FormPicker) commitMenu() {
	index := picker.fm.menu.GetCurrentItem()
	picker.closeMenu()
	picker.SetCurrentOption(index)
}

func (fm *FormModal) focusedPicker() *FormPicker {
	view, ok := fm.focusedPrimitive().(*tview.TextView)
	if !ok {
		return nil
	}
	return fm.pickers[view]
}

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
	screenW, screenH := screen.Size()
	longest := 0
	for _, option := range picker.options {
		if width := len(option); width > longest {
			longest = width
		}
	}
	x, y, width, height, fits := pickerMenuRect(fieldX, fieldY, fieldWidth, fieldHeight, len(picker.options), longest, screenW, screenH)
	if !fits {
		picker.closeMenu()
		return
	}

	fm.menu.SetRect(x, y, width, height)
	fm.menu.Draw(screen)
}

func pickerMenuRect(fieldX, fieldY, fieldWidth, fieldHeight, optionCount, longestOption, screenW, screenH int) (x, y, width, height int, fits bool) {
	rows := optionCount
	if rows > formPickerMenuRows {
		rows = formPickerMenuRows
	}
	if fieldWidth < 3 || rows < 1 {
		return 0, 0, 0, 0, false
	}

	width = fieldWidth
	if wanted := longestOption + 2; wanted > width {
		width = wanted
	}
	x = fieldX
	if x+width > screenW {
		if x = screenW - width; x < 0 {
			x, width = 0, screenW
		}
	}

	height = rows + 2
	below := fieldY + fieldHeight - 1
	above := fieldY - height + 1
	switch {
	case below+height <= screenH:
		y = below
	case above >= 0:
		y = above
	default:
		roomBelow := screenH - below
		roomAbove := fieldY + 1
		if roomBelow >= roomAbove {
			y, height = below, roomBelow
		} else {
			y, height = 0, roomAbove
		}
		if height < 3 {
			return 0, 0, 0, 0, false
		}
	}
	return x, y, width, height, true
}
