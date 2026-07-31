package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	formModalDefaultMaxWidth = 76
	formModalScreenHMargin   = 4
	formModalScreenWMargin   = 8
	formTextAreaMinRows      = 3
)

// FormButton is one action in a FormModal's button row.
type FormButton struct {
	Label   string
	OnPress func()
}

// formRow is one vertical unit in the form: a caps label plus its widget(s).
type formRow struct {
	container  *tview.Flex
	height     int
	minHeight  int
	flexible   bool
	focusables []tview.Primitive
	frame      *tview.Flex
	labelView  *tview.TextView
}

// FormModal renders a Linear-style form modal: caps labels above framed
// fields, content-driven sizing, focus-only accent buttons, a dim hint line.
// It owns the shell, tab order, and shared keys so individual modals only
// declare fields and callbacks.
type FormModal struct {
	app        *App
	title      string
	maxWidth   int
	root       *tview.Flex
	frame      *tview.Flex
	rowsBox    *tview.Flex
	buttonsRow *tview.Flex
	hintView   *tview.TextView
	rows       []formRow
	order      []tview.Primitive
	buttons    []*tview.Button
	pickerRow  *pickerRowState
	focusIdx   int
	scrollTop  int
	onCancel   func()
	onSubmit   func()
}

// pickerRowState tracks the open shared row consecutive pickers pack into.
type pickerRowState struct {
	labels *tview.Flex
	values *tview.Flex
	rowIdx int
}

// NewFormModal creates an empty form modal shell with the given border title.
func NewFormModal(app *App, title string) *FormModal {
	fm := &FormModal{app: app, title: title}

	fm.rowsBox = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.rowsBox.SetBackgroundColor(app.theme.ModalBackground())

	fm.hintView = tview.NewTextView()
	fm.hintView.SetTextColor(app.theme.SecondaryText)
	fm.hintView.SetBackgroundColor(app.theme.ModalBackground())

	fm.frame = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.frame.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.BorderFocus).
		SetTitle(" " + title + " ").
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	fm.frame.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	fm.frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return fm.HandleKey(event)
	})

	fm.root = tview.NewFlex()
	fm.root.SetBackgroundColor(app.theme.Background)

	return fm
}

// SetMaxWidth overrides the default width clamp (76).
func (fm *FormModal) SetMaxWidth(w int) { fm.maxWidth = w }

// SetTitle updates the border title (some modals retitle per Show).
func (fm *FormModal) SetTitle(title string) {
	fm.title = title
	fm.frame.SetTitle(" " + title + " ")
}

// SetHint sets the dim hint line inside the bottom border.
func (fm *FormModal) SetHint(hint string) { fm.hintView.SetText(hint) }

// SetRowLabel retitles a field's caps label (some modals relabel per Show).
func (fm *FormModal) SetRowLabel(rowIdx int, label string) {
	if rowIdx >= 0 && rowIdx < len(fm.rows) && fm.rows[rowIdx].labelView != nil {
		fm.rows[rowIdx].labelView.SetText(strings.ToUpper(label))
	}
}

// SetOnCancel sets the Esc handler.
func (fm *FormModal) SetOnCancel(fn func()) { fm.onCancel = fn }

// SetOnSubmit sets the Ctrl+Enter / Cmd+Enter handler.
func (fm *FormModal) SetOnSubmit(fn func()) { fm.onSubmit = fn }

// AddInput appends a single-line text field under a caps label.
func (fm *FormModal) AddInput(label, initial string) *tview.InputField {
	input := tview.NewInputField().
		SetFieldBackgroundColor(fm.app.theme.ModalBackground()).
		SetFieldTextColor(fm.app.theme.Foreground).
		SetFieldWidth(0).
		SetText(initial)
	input.SetBackgroundColor(fm.app.theme.ModalBackground())
	fm.addFramedRow(label, input, 1, false)
	return input
}

// AddTextArea appends a multi-line text field under a caps label. rows is the
// preferred height; the row shrinks first when the screen is short.
func (fm *FormModal) AddTextArea(label, initial string, rows int) *tview.TextArea {
	area := tview.NewTextArea().
		SetTextStyle(tcell.StyleDefault.
			Foreground(fm.app.theme.Foreground).
			Background(fm.app.theme.ModalBackground()))
	area.SetText(initial, false)
	area.SetBackgroundColor(fm.app.theme.ModalBackground())
	fm.addFramedRow(label, area, rows, true)
	return area
}

// AddPicker appends a dropdown. Consecutive AddPicker calls share one
// two-row unit: caps labels on the first row, the dropdowns beneath, equal
// widths — the New Issue assignee/cycle/priority row.
func (fm *FormModal) AddPicker(label string, options []string, selected int, onChange func(text string, index int)) *tview.DropDown {
	theme := fm.app.theme

	dd := tview.NewDropDown().
		SetOptions(options, onChange).
		SetFieldBackgroundColor(theme.ModalBackground()).
		SetFieldTextColor(theme.Foreground)
	dd.SetFieldWidth(0)
	dd.SetListStyles(
		tcell.StyleDefault.Background(theme.ModalBackground()).Foreground(theme.Foreground),
		tcell.StyleDefault.Background(theme.Accent).Foreground(theme.InverseTextColor()),
	)
	dd.SetBackgroundColor(theme.ModalBackground())
	if selected >= 0 && selected < len(options) {
		dd.SetCurrentOption(selected)
	}

	labelView := tview.NewTextView()
	labelView.SetText(strings.ToUpper(label))
	labelView.SetTextColor(theme.SecondaryText)
	labelView.SetBackgroundColor(theme.ModalBackground())

	if fm.pickerRow == nil {
		labels := tview.NewFlex()
		labels.SetBackgroundColor(theme.ModalBackground())
		values := tview.NewFlex()
		values.SetBackgroundColor(theme.ModalBackground())

		container := tview.NewFlex().SetDirection(tview.FlexRow)
		container.SetBackgroundColor(theme.ModalBackground())
		container.AddItem(labels, 1, 0, false)
		container.AddItem(values, 1, 0, true)

		fm.appendRow(formRow{
			container: container,
			height:    2,
			minHeight: 2,
		})
		fm.pickerRow = &pickerRowState{labels: labels, values: values, rowIdx: len(fm.rows) - 1}
	} else {
		fm.pickerRow.labels.AddItem(nil, 2, 0, false)
		fm.pickerRow.values.AddItem(nil, 2, 0, false)
	}

	fm.pickerRow.labels.AddItem(labelView, 0, 1, false)
	fm.pickerRow.values.AddItem(dd, 0, 1, true)
	rowIdx := fm.pickerRow.rowIdx
	fm.rows[rowIdx].focusables = append(fm.rows[rowIdx].focusables, dd)
	fm.registerFocusable(dd, rowIdx)
	return dd
}

// AddStatic appends a one-row read-only line (e.g. the sub-issue parent).
func (fm *FormModal) AddStatic(text string) *tview.TextView {
	view := tview.NewTextView()
	view.SetText(text)
	view.SetTextColor(fm.app.theme.SecondaryText)
	view.SetBackgroundColor(fm.app.theme.ModalBackground())
	fm.pickerRow = nil
	fm.appendRow(formRow{container: staticRowContainer(view, fm.app.theme), height: 1, minHeight: 1})
	return view
}

// staticRowContainer wraps a static view so rows stay uniform Flex items.
func staticRowContainer(view *tview.TextView, theme Theme) *tview.Flex {
	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBackgroundColor(theme.ModalBackground())
	container.AddItem(view, 1, 0, false)
	return container
}

// addFramedRow builds the caps-label-plus-framed-editor unit shared by text
// fields and registers the editor in the tab order.
func (fm *FormModal) addFramedRow(label string, editor tview.Primitive, editorRows int, flexible bool) {
	fm.pickerRow = nil
	theme := fm.app.theme

	labelView := tview.NewTextView()
	labelView.SetText(strings.ToUpper(label))
	labelView.SetTextColor(theme.SecondaryText)
	labelView.SetBackgroundColor(theme.ModalBackground())

	editorFrame := tview.NewFlex().SetDirection(tview.FlexRow)
	editorFrame.AddItem(editor, 0, 1, true)
	editorFrame.SetBackgroundColor(theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(theme.Border)

	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBackgroundColor(theme.ModalBackground())
	container.AddItem(labelView, 1, 0, false)
	container.AddItem(editorFrame, 0, 1, true)

	row := formRow{
		container:  container,
		height:     1 + editorRows + 2,
		minHeight:  1 + formTextAreaMinRows + 2,
		flexible:   flexible,
		focusables: []tview.Primitive{editor},
		frame:      editorFrame,
		labelView:  labelView,
	}
	if !flexible {
		row.minHeight = row.height
	}
	fm.appendRow(row)
	fm.registerFocusable(editor, len(fm.rows)-1)
}

// appendRow adds a row to the rows container at its full height.
func (fm *FormModal) appendRow(row formRow) {
	fm.rows = append(fm.rows, row)
	fm.rowsBox.AddItem(row.container, row.height, 0, len(row.focusables) > 0)
}

// registerFocusable wires a widget into the tab order and focus styling.
func (fm *FormModal) registerFocusable(p tview.Primitive, rowIdx int) {
	fm.order = append(fm.order, p)
	if box, ok := p.(interface{ SetFocusFunc(func()) *tview.Box }); ok {
		box.SetFocusFunc(func() { fm.onFocused(p, rowIdx) })
	}
}

// onFocused tracks focus for tab cycling, recolors field frames, and keeps
// the focused row visible when the form scrolls.
func (fm *FormModal) onFocused(p tview.Primitive, rowIdx int) {
	for i, candidate := range fm.order {
		if candidate == p {
			fm.focusIdx = i
			break
		}
	}
	for _, row := range fm.rows {
		if row.frame == nil {
			continue
		}
		focused := false
		for _, f := range row.focusables {
			if f == p {
				focused = true
				break
			}
		}
		if focused {
			row.frame.SetBorderColor(fm.app.theme.BorderFocus)
		} else {
			row.frame.SetBorderColor(fm.app.theme.Border)
		}
	}
	// Pickers have no frame; the focused one gets the input fill instead.
	for _, candidate := range fm.order {
		if dd, ok := candidate.(*tview.DropDown); ok {
			if dd == p {
				dd.SetFieldBackgroundColor(fm.app.theme.InputBg)
			} else {
				dd.SetFieldBackgroundColor(fm.app.theme.ModalBackground())
			}
		}
	}
	if rowIdx >= 0 {
		fm.ensureVisible(rowIdx)
	}
}

// AddButtons appends the action row. With focus-only accent styling there is
// no persistent primary; order carries the emphasis and Cancel goes last.
func (fm *FormModal) AddButtons(buttons ...FormButton) {
	theme := fm.app.theme
	fm.pickerRow = nil
	fm.buttonsRow = tview.NewFlex()
	fm.buttonsRow.SetBackgroundColor(theme.ModalBackground())
	for i, spec := range buttons {
		if i > 0 {
			fm.buttonsRow.AddItem(nil, 3, 0, false)
		}
		btn := tview.NewButton(spec.Label)
		btn.SetStyle(tcell.StyleDefault.
			Foreground(theme.SecondaryText).
			Background(theme.ModalBackground()))
		btn.SetActivatedStyle(tcell.StyleDefault.
			Foreground(theme.InverseTextColor()).
			Background(theme.Accent))
		if press := spec.OnPress; press != nil {
			btn.SetSelectedFunc(press)
		}
		fm.buttonsRow.AddItem(btn, len(spec.Label)+4, 0, false)
		fm.buttons = append(fm.buttons, btn)
		fm.registerFocusable(btn, -1)
	}
	fm.buttonsRow.AddItem(nil, 0, 1, false)
}

// effectiveMaxWidth returns the configured or default width clamp.
func (fm *FormModal) effectiveMaxWidth() int {
	if fm.maxWidth > 0 {
		return fm.maxWidth
	}
	return formModalDefaultMaxWidth
}

// chromeHeight counts the non-row lines inside the modal: border, padding,
// the blank-plus-buttons block when present, and the always-reserved hint.
func (fm *FormModal) chromeHeight() int {
	padding := fm.app.density.ModalPadding
	chrome := 2 + padding.Top + padding.Bottom + 1 // border + padding + hint
	if fm.buttonsRow != nil {
		chrome += 2 // blank spacer + button row
	}
	return chrome
}

// rowHeights returns per-row heights after shrinking flexible rows to fit
// the screen clamp. When even the floor overflows, the window scrolls.
func (fm *FormModal) rowHeights(screenH int) []int {
	heights := make([]int, len(fm.rows))
	total := fm.chromeHeight()
	for i, row := range fm.rows {
		heights[i] = row.height
		total += row.height
	}
	maxHeight := screenH - formModalScreenHMargin
	need := total - maxHeight
	for need > 0 {
		shrunk := false
		for i, row := range fm.rows {
			if need <= 0 {
				break
			}
			if row.flexible && heights[i] > row.minHeight {
				heights[i]--
				need--
				shrunk = true
			}
		}
		if !shrunk {
			break
		}
	}
	return heights
}

// contentHeight is the modal's total height for the given screen height:
// content-fit, clamped to the screen.
func (fm *FormModal) contentHeight(screenH int) int {
	total := fm.chromeHeight()
	for _, h := range fm.rowHeights(screenH) {
		total += h
	}
	if maxHeight := screenH - formModalScreenHMargin; total > maxHeight {
		return maxHeight
	}
	return total
}

// ensureVisible scrolls the row window so the given row is fully on screen.
func (fm *FormModal) ensureVisible(rowIdx int) {
	_, _, _, screenH := fm.app.pages.GetRect()
	heights := fm.rowHeights(screenH)
	avail := fm.contentHeight(screenH) - fm.chromeHeight()

	if rowIdx < fm.scrollTop {
		fm.scrollTop = rowIdx
	}
	for fm.scrollTop < rowIdx {
		used := 0
		for i := fm.scrollTop; i <= rowIdx; i++ {
			used += heights[i]
		}
		if used <= avail {
			break
		}
		fm.scrollTop++
	}
	fm.applyRowWindow(heights, avail)
}

// applyRowWindow resizes rows so only the scroll window occupies space.
func (fm *FormModal) applyRowWindow(heights []int, avail int) {
	used := 0
	for i, row := range fm.rows {
		h := 0
		if i >= fm.scrollTop && used+heights[i] <= avail {
			h = heights[i]
			used += h
		}
		fm.rowsBox.ResizeItem(row.container, h, 0)
	}
}

// layout sizes the modal for the current screen and rebuilds the centering
// wrappers. Pointers stay stable so pages keep referencing the same root.
func (fm *FormModal) layout() {
	_, _, screenW, screenH := fm.app.pages.GetRect()

	width := screenW - formModalScreenWMargin
	if max := fm.effectiveMaxWidth(); width > max || width <= 0 {
		width = max
	}
	height := fm.contentHeight(screenH)

	heights := fm.rowHeights(screenH)
	rowsTotal := 0
	for _, h := range heights {
		rowsTotal += h
	}
	avail := height - fm.chromeHeight()
	fm.applyRowWindow(heights, avail)

	fm.frame.Clear()
	fm.frame.AddItem(fm.rowsBox, 0, 1, true)
	if fm.buttonsRow != nil {
		fm.frame.AddItem(nil, 1, 0, false)
		fm.frame.AddItem(fm.buttonsRow, 1, 0, false)
	}
	fm.frame.AddItem(fm.hintView, 1, 0, false)

	column := tview.NewFlex().SetDirection(tview.FlexRow)
	column.AddItem(nil, 0, 1, false)
	column.AddItem(fm.frame, height, 0, true)
	column.AddItem(nil, 0, 1, false)

	fm.root.Clear()
	fm.root.AddItem(nil, 0, 1, false)
	fm.root.AddItem(column, width, 0, true)
	fm.root.AddItem(nil, 0, 1, false)
}

// Show lays the modal out for the current screen, resets focus to the first
// field (tview primitives remember focus across shows), and raises the page.
func (fm *FormModal) Show(pageName string) {
	fm.scrollTop = 0
	fm.focusIdx = 0
	fm.layout()
	fm.app.pages.AddPage(pageName, fm.root, true, true)
	fm.app.pages.SendToFront(pageName)
	if len(fm.order) > 0 {
		fm.app.app.SetFocus(fm.order[0])
	}
}

// Hide removes the page and restores pane focus.
func (fm *FormModal) Hide(pageName string) {
	fm.app.pages.RemovePage(pageName)
	fm.app.updateFocus()
}

// Root returns the fullscreen wrapper for pages.
func (fm *FormModal) Root() *tview.Flex { return fm.root }

// ContentBody returns the rows-plus-buttons column without the modal shell,
// for modals that compose the form beside other panes (prompt templates).
// The embedding modal owns the border, sizing, and hint line.
func (fm *FormModal) ContentBody() *tview.Flex {
	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(fm.app.theme.ModalBackground())
	body.AddItem(fm.rowsBox, 0, 1, true)
	if fm.buttonsRow != nil {
		body.AddItem(nil, 1, 0, false)
		body.AddItem(fm.buttonsRow, 1, 0, false)
	}
	return body
}

// Focus returns keyboard focus to the form's current field — used when an
// overlay (e.g. a picker) closes over a stacked form modal.
func (fm *FormModal) Focus() {
	if p := fm.focusedPrimitive(); p != nil {
		fm.app.app.SetFocus(p)
	}
}

// openDropdown returns the open dropdown in the tab order, if any.
func (fm *FormModal) openDropdown() *tview.DropDown {
	for _, p := range fm.order {
		if dd, ok := p.(*tview.DropDown); ok && dd.IsOpen() {
			return dd
		}
	}
	return nil
}

// focusStep moves keyboard focus through the tab order, wrapping.
func (fm *FormModal) focusStep(delta int) {
	if len(fm.order) == 0 {
		return
	}
	fm.focusIdx = (fm.focusIdx + delta + len(fm.order)) % len(fm.order)
	fm.app.app.SetFocus(fm.order[fm.focusIdx])
}

// HandleKey implements the shared form keys. It is called from the app-level
// modal dispatcher (a parent's InputCapture never sees keys sent to a focused
// child, so routing lives here).
func (fm *FormModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if dd := fm.openDropdown(); dd != nil {
		if event.Key() == tcell.KeyEscape {
			if handler := dd.InputHandler(); handler != nil {
				handler(event, func(p tview.Primitive) { fm.app.app.SetFocus(p) })
			}
			return nil
		}
		return event
	}

	switch event.Key() {
	case tcell.KeyEscape:
		if fm.onCancel != nil {
			fm.onCancel()
		}
		return nil
	case tcell.KeyTab:
		fm.focusStep(1)
		return nil
	case tcell.KeyBacktab:
		fm.focusStep(-1)
		return nil
	case tcell.KeyEnter:
		if event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			if fm.onSubmit != nil {
				fm.onSubmit()
			}
			return nil
		}
		if _, ok := fm.focusedPrimitive().(*tview.InputField); ok {
			fm.focusStep(1)
			return nil
		}
	}
	return event
}

// focusedPrimitive returns the widget the tab order considers focused.
func (fm *FormModal) focusedPrimitive() tview.Primitive {
	if fm.focusIdx >= 0 && fm.focusIdx < len(fm.order) {
		return fm.order[fm.focusIdx]
	}
	return nil
}
