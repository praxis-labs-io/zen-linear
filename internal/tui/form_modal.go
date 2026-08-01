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
	frameOf    map[tview.Primitive]*tview.Flex
	pickerMeta map[*tview.DropDown]*pickerState
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
	dds    []*tview.DropDown
}

// pickerState remembers a dropdown's options and layout width so the field
// and its menu can be sized to the frame (tview sizes both to the longest
// option by default).
type pickerState struct {
	options    []string
	fieldWidth int
}

// NewFormModal creates an empty form modal shell with the given border title.
func NewFormModal(app *App, title string) *FormModal {
	fm := &FormModal{
		app:        app,
		title:      title,
		frameOf:    make(map[tview.Primitive]*tview.Flex),
		pickerMeta: make(map[*tview.DropDown]*pickerState),
	}

	fm.rowsBox = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.rowsBox.SetBackgroundColor(app.theme.ModalBackground())

	fm.hintView = tview.NewTextView()
	fm.hintView.SetTextColor(app.theme.SecondaryText)
	fm.hintView.SetBackgroundColor(app.theme.ModalBackground())
	fm.hintView.SetTextAlign(tview.AlignCenter)

	fm.frame = tview.NewFlex().SetDirection(tview.FlexRow)
	// tview.NewFlex marks its Box dontClear, leaving the layer beneath
	// visible through every cell the children don't paint. A fresh Box
	// restores the background fill so the modal is opaque like the palette.
	fm.frame.Box = tview.NewBox()
	fm.frame.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.BorderFocus).
		SetTitle(" " + title + " ").
		SetTitleColor(app.theme.Foreground)
	// Vertical space comes from the row layout itself; only the horizontal
	// padding is kept so fields don't touch the border.
	padding := app.density.ModalPadding
	fm.frame.SetBorderPadding(0, 0, padding.Left, padding.Right)
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
	// The closed-but-focused field has its own style, defaulting to the
	// bright tview palette that swallows the text on themed backgrounds.
	// The frame border carries the focus cue, like the text fields.
	dd.SetFocusedStyle(tcell.StyleDefault.
		Background(theme.ModalBackground()).
		Foreground(theme.Foreground))
	dd.SetFieldWidth(0)
	// The open menu has no border access, so a contrasting fill sets it
	// apart from the panel instead.
	dd.SetListStyles(
		tcell.StyleDefault.Background(theme.InputBg).Foreground(theme.Foreground),
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

	frame := tview.NewFlex().SetDirection(tview.FlexRow)
	frame.Box = tview.NewBox() // restore the background fill (see NewFormModal)
	frame.AddItem(dd, 0, 1, true)
	frame.SetBackgroundColor(theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(theme.Border)

	if fm.pickerRow == nil {
		labels := tview.NewFlex()
		labels.SetBackgroundColor(theme.ModalBackground())
		values := tview.NewFlex()
		values.SetBackgroundColor(theme.ModalBackground())

		container := tview.NewFlex().SetDirection(tview.FlexRow)
		container.SetBackgroundColor(theme.ModalBackground())
		container.AddItem(labels, 1, 0, false)
		container.AddItem(values, 3, 0, true)

		fm.appendRow(formRow{
			container: container,
			height:    4,
			minHeight: 4,
		})
		fm.pickerRow = &pickerRowState{labels: labels, values: values, rowIdx: len(fm.rows) - 1}
	} else {
		fm.pickerRow.labels.AddItem(nil, 2, 0, false)
		fm.pickerRow.values.AddItem(nil, 2, 0, false)
	}

	fm.pickerRow.labels.AddItem(labelView, 0, 1, false)
	fm.pickerRow.values.AddItem(frame, 0, 1, true)
	fm.pickerRow.dds = append(fm.pickerRow.dds, dd)
	rowIdx := fm.pickerRow.rowIdx
	fm.rows[rowIdx].focusables = append(fm.rows[rowIdx].focusables, dd)
	fm.frameOf[dd] = frame
	fm.pickerMeta[dd] = &pickerState{options: append([]string(nil), options...)}
	fm.registerFocusable(dd, rowIdx)
	return dd
}

// SetPickerOptions replaces a picker's options. Modals must use this instead
// of DropDown.SetOptions so the field and menu keep filling the frame.
func (fm *FormModal) SetPickerOptions(dd *tview.DropDown, options []string, onChange func(text string, index int)) {
	dd.SetOptions(options, onChange)
	if meta, ok := fm.pickerMeta[dd]; ok {
		meta.options = append([]string(nil), options...)
		fm.applyPickerWidth(dd, meta)
	}
}

// applyPickerWidth sizes the closed field to the frame and pads the menu
// rows out to the same width (tview offers no direct menu-width control).
func (fm *FormModal) applyPickerWidth(dd *tview.DropDown, meta *pickerState) {
	if meta.fieldWidth <= 0 {
		return
	}
	dd.SetFieldWidth(meta.fieldWidth)
	longest := 0
	for _, option := range meta.options {
		if w := len(option); w > longest {
			longest = w
		}
	}
	pad := meta.fieldWidth - longest
	if pad < 0 {
		pad = 0
	}
	dd.SetTextOptions("", strings.Repeat(" ", pad), "", "", "")
}

// layoutPickers computes each picker's frame-interior width for the current
// modal width and applies it. Called from layout().
func (fm *FormModal) layoutPickers(modalWidth int) {
	padding := fm.app.density.ModalPadding
	rowInner := modalWidth - 2 - padding.Left - padding.Right
	for _, row := range fm.rows {
		var dds []*tview.DropDown
		for _, focusable := range row.focusables {
			if dd, ok := focusable.(*tview.DropDown); ok {
				dds = append(dds, dd)
			}
		}
		if len(dds) == 0 {
			continue
		}
		// Columns split the row evenly with 2-cell gaps; each frame spends
		// 2 more cells on its border.
		colWidth := (rowInner-2*(len(dds)-1))/len(dds) - 2
		if colWidth < 1 {
			colWidth = 1
		}
		for _, dd := range dds {
			if meta, ok := fm.pickerMeta[dd]; ok {
				meta.fieldWidth = colWidth
				fm.applyPickerWidth(dd, meta)
			}
		}
	}
}

// AddCheckbox appends a toggle in the same caps-label-over-framed-field
// unit as every other control.
func (fm *FormModal) AddCheckbox(label string, checked bool) *tview.Checkbox {
	theme := fm.app.theme

	box := tview.NewCheckbox().SetChecked(checked)
	fieldStyle := tcell.StyleDefault.Background(theme.InputBg).Foreground(theme.Foreground)
	box.SetUncheckedStyle(fieldStyle)
	box.SetCheckedStyle(fieldStyle)
	// Like DropDown, the focused checkbox has its own style with a bright
	// unusable default; the accent marks focus, matching the buttons.
	box.SetActivatedStyle(tcell.StyleDefault.
		Background(theme.Accent).
		Foreground(theme.InverseTextColor()))
	box.SetBackgroundColor(theme.ModalBackground())

	fm.addFramedRow(label, box, 1, false)
	return box
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
	editorFrame.Box = tview.NewBox() // restore the background fill (see NewFormModal)
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
		labelView:  labelView,
	}
	if !flexible {
		row.minHeight = row.height
	}
	fm.appendRow(row)
	fm.frameOf[editor] = editorFrame
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
	for widget, frame := range fm.frameOf {
		if widget == p {
			frame.SetBorderColor(fm.app.theme.BorderFocus)
		} else {
			frame.SetBorderColor(fm.app.theme.Border)
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
	fm.buttonsRow.AddItem(nil, 0, 1, false)
	for i, spec := range buttons {
		if i > 0 {
			fm.buttonsRow.AddItem(nil, 3, 0, false)
		}
		btn := tview.NewButton(spec.Label)
		btn.SetStyle(tcell.StyleDefault.
			Foreground(theme.Foreground).
			Background(theme.InputBg))
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

// chromeHeight counts the non-row lines inside the modal: border, the
// blank-plus-buttons block when present, and the gap plus hint line.
func (fm *FormModal) chromeHeight() int {
	chrome := 2 + 1 + 1 // border + gap + hint
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

// screenSize returns the pages' current width and height.
func (fm *FormModal) screenSize() (int, int) {
	_, _, w, h := fm.app.pages.GetRect()
	return w, h
}

// ensureVisible scrolls the row window so the given row is fully on screen.
func (fm *FormModal) ensureVisible(rowIdx int) {
	_, screenH := fm.screenSize()
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
	screenW, screenH := fm.screenSize()

	width := screenW - formModalScreenWMargin
	if limit := fm.effectiveMaxWidth(); width > limit || width <= 0 {
		width = limit
	}
	height := fm.contentHeight(screenH)
	fm.layoutPickers(width)

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
	fm.frame.AddItem(nil, 1, 0, false)
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

// BlurFrames clears every field's focus highlight — for composed modals
// where focus can leave the form (e.g. the prompt-templates list).
func (fm *FormModal) BlurFrames() {
	for _, frame := range fm.frameOf {
		frame.SetBorderColor(fm.app.theme.Border)
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

// focusRowStep moves keyboard focus vertically: to the first widget of the
// next or previous row, with the button row as the last stop. Widgets that
// share a row (pickers, buttons) are one stop, not several.
func (fm *FormModal) focusRowStep(delta int) {
	var stops []tview.Primitive
	for _, row := range fm.rows {
		if len(row.focusables) > 0 {
			stops = append(stops, row.focusables[0])
		}
	}
	if len(fm.buttons) > 0 {
		stops = append(stops, fm.buttons[0])
	}
	if len(stops) == 0 {
		return
	}

	current := 0
	focused := fm.focusedPrimitive()
	if _, isButton := focused.(*tview.Button); isButton {
		current = len(stops) - 1
	} else {
	rows:
		for _, row := range fm.rows {
			for _, f := range row.focusables {
				if f == focused {
					break rows
				}
			}
			if len(row.focusables) > 0 {
				current++
			}
		}
	}

	next := (current + delta + len(stops)) % len(stops)
	fm.app.app.SetFocus(stops[next])
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
	case tcell.KeyDown:
		// Textareas keep vertical arrows for cursor movement until the
		// cursor sits on the boundary line; then the arrow leaves the field.
		if ta, ok := fm.focusedPrimitive().(*tview.TextArea); ok {
			if textAreaCursorOnLastLine(ta) {
				fm.focusRowStep(1)
				return nil
			}
			return event
		}
		fm.focusRowStep(1)
		return nil
	case tcell.KeyUp:
		if ta, ok := fm.focusedPrimitive().(*tview.TextArea); ok {
			if textAreaCursorOnFirstLine(ta) {
				fm.focusRowStep(-1)
				return nil
			}
			return event
		}
		fm.focusRowStep(-1)
		return nil
	case tcell.KeyLeft:
		// Buttons and closed pickers navigate sideways; text fields keep
		// horizontal arrows for the cursor.
		switch fm.focusedPrimitive().(type) {
		case *tview.Button, *tview.DropDown:
			fm.focusStep(-1)
			return nil
		}
	case tcell.KeyRight:
		switch fm.focusedPrimitive().(type) {
		case *tview.Button, *tview.DropDown:
			fm.focusStep(1)
			return nil
		}
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

// textAreaCursorRow returns the line the cursor selection starts on.
func textAreaCursorRow(ta *tview.TextArea) int {
	fromRow, fromColumn, toRow, toColumn := ta.GetCursor()
	_ = fromColumn
	_ = toRow
	_ = toColumn
	return fromRow
}

// textAreaCursorOnFirstLine reports whether the cursor sits on line one.
func textAreaCursorOnFirstLine(ta *tview.TextArea) bool {
	return textAreaCursorRow(ta) == 0
}

// textAreaCursorOnLastLine reports whether the cursor sits on the last line.
func textAreaCursorOnLastLine(ta *tview.TextArea) bool {
	return textAreaCursorRow(ta) >= strings.Count(ta.GetText(), "\n")
}

// focusedPrimitive returns the widget the tab order considers focused.
func (fm *FormModal) focusedPrimitive() tview.Primitive {
	if fm.focusIdx >= 0 && fm.focusIdx < len(fm.order) {
		return fm.order[fm.focusIdx]
	}
	return nil
}
