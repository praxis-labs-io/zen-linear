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
	formFieldRows            = 4
	packedColumnGap          = 2
	packedLabelBudget        = 16
	sectionRailSpacerRows    = 1
	railTopPad               = 1
	sectionRailPad           = 2
	formRowsMinWidth         = 34
)

type FormButton struct {
	Label   string
	OnPress func()
}

type formRow struct {
	container  *tview.Flex
	height     int
	minHeight  int
	columns    int
	flexible   bool
	hidden     bool
	focusables []tview.Primitive
	labelView  *tview.TextView
	packed     []packedColumn
	perLine    int
	section    int
}

type formSection struct {
	name string
}

type packedColumn struct {
	labelView *tview.TextView
	frame     *tview.Flex
	labelLen  int
}

type FormModal struct {
	app            *App
	title          string
	maxWidth       int
	root           *tview.Flex
	frame          *tview.Flex
	rowsBox        *tview.Flex
	buttonsRow     *tview.Flex
	hintView       *tview.TextView
	contextView    *tview.TextView
	contextText    string
	hintText       string
	rows           []formRow
	order          []tview.Primitive
	buttons        []*tview.Button
	buttonLabels   []string
	frameOf        map[tview.Primitive]*tview.Flex
	checkboxLabels map[*tview.Checkbox]*tview.TextView
	multiSelects   map[*tview.List]*FormMultiSelect
	pickers        map[*tview.TextView]*FormPicker
	locked         map[tview.Primitive]bool
	rowOf          map[tview.Primitive]int
	openPicker     *FormPicker
	menu           *tview.List
	page           *formPage
	pickerRow      *pickerRowState
	sections       []formSection
	activeSection  int
	sectionRail    *tview.TextView
	railRule       *tview.Box
	footerRule     *tview.Box
	navColumn      *tview.Flex
	buttonsColumn  *tview.Flex
	railFocused    bool
	body           *tview.Flex
	focusIdx       int
	initialFocus   tview.Primitive
	scrollTop      int
	scrollAbove    bool
	scrollBelow    bool
	onCancel       func()
	onSubmit       func()
}

// A Flex has no z-order, so an open picker menu cannot be one of the form's rows.
type formPage struct {
	*tview.Flex
	fm *FormModal
}

func (p *formPage) Draw(screen tcell.Screen) {
	p.Flex.Draw(screen)
	p.fm.drawOpenMenu(screen)
}

type pickerRowState struct {
	rowIdx int
}

func NewFormModal(app *App, title string) *FormModal {
	fm := &FormModal{
		app:            app,
		title:          title,
		frameOf:        make(map[tview.Primitive]*tview.Flex),
		checkboxLabels: make(map[*tview.Checkbox]*tview.TextView),
		multiSelects:   make(map[*tview.List]*FormMultiSelect),
		pickers:        make(map[*tview.TextView]*FormPicker),
		locked:         make(map[tview.Primitive]bool),
		rowOf:          make(map[tview.Primitive]int),
	}

	fm.rowsBox = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.rowsBox.SetBackgroundColor(app.theme.ModalBackground())

	fm.body = tview.NewFlex()
	fm.body.SetBackgroundColor(app.theme.ModalBackground())

	fm.hintView = tview.NewTextView()
	fm.hintView.SetTextColor(app.theme.SecondaryText)
	fm.hintView.SetBackgroundColor(app.theme.ModalBackground())
	fm.hintView.SetTextAlign(tview.AlignCenter)

	fm.contextView = tview.NewTextView()
	fm.contextView.SetWrap(false)
	fm.contextView.SetDynamicColors(true)
	fm.contextView.SetBackgroundColor(app.theme.ModalBackground())

	fm.frame = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.frame.Box = tview.NewBox()
	fm.frame.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.BorderFocus).
		SetTitle(" " + title + " ").
		SetTitleColor(app.theme.Accent)
	padding := app.density.ModalPadding
	fm.frame.SetBorderPadding(0, 0, padding.Left, padding.Right)
	fm.frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return fm.HandleKey(event)
	})
	fm.frame.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		marker := tcell.StyleDefault.
			Background(app.theme.ModalBackground()).
			Foreground(app.theme.SecondaryText)
		if fm.scrollAbove {
			screen.SetContent(x+width-3, y, '↑', nil, marker)
		}
		if fm.scrollBelow {
			screen.SetContent(x+width-3, y+height-1, '↓', nil, marker)
		}
		fm.renderSectionRail()
		return fm.frame.GetInnerRect()
	})

	fm.menu = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.Background(app.theme.ModalBackground()).Foreground(app.theme.Foreground)).
		SetSelectedStyle(tcell.StyleDefault.Background(app.theme.Accent).Foreground(app.theme.InverseTextColor())).
		SetHighlightFullLine(true)
	fm.menu.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.BorderFocus)

	fm.root = tview.NewFlex()
	fm.root.SetBackgroundColor(app.theme.Background)
	fm.page = &formPage{Flex: fm.root, fm: fm}

	return fm
}

func (fm *FormModal) SetMaxWidth(w int) { fm.maxWidth = w }

func (fm *FormModal) SetTitle(title string) {
	fm.title = title
	fm.frame.SetTitle(" " + title + " ")
}

func (fm *FormModal) SetHint(hint string) {
	fm.hintText = hint
	fm.hintView.SetTextColor(fm.app.theme.SecondaryText)
	fm.hintView.SetText(hint)
}

// SetStatus replaces the hint line with a message about the form; an empty message restores the hint.
func (fm *FormModal) SetStatus(message string, isError bool) {
	if message == "" {
		fm.SetHint(fm.hintText)
		return
	}
	color := fm.app.theme.SecondaryText
	if isError {
		color = fm.app.theme.StatusCanceled
	}
	fm.hintView.SetTextColor(color)
	fm.hintView.SetText(message)
}

// SetLocked makes a field read-only while it still takes focus and stays readable.
func (fm *FormModal) SetLocked(p tview.Primitive, locked bool) {
	fm.locked[p] = locked

	color := fm.app.theme.Foreground
	if locked {
		color = fm.app.theme.SecondaryText
	}
	switch field := p.(type) {
	case *tview.InputField:
		field.SetAcceptanceFunc(func(string, rune) bool { return !fm.locked[p] })
		field.SetFieldTextColor(color)
	case *tview.TextView:
		field.SetTextColor(color)
	}
}

func (fm *FormModal) isLocked(p tview.Primitive) bool { return fm.locked[p] }

// SetContext sets the line pinned above the fields; an empty string hides it.
func (fm *FormModal) SetContext(text string) {
	fm.contextText = text
	fm.contextView.SetText(text)
}

func (fm *FormModal) SetRowLabel(rowIdx int, label string) {
	if rowIdx >= 0 && rowIdx < len(fm.rows) && fm.rows[rowIdx].labelView != nil {
		fm.rows[rowIdx].labelView.SetText(strings.ToUpper(label))
	}
}

func (fm *FormModal) RowCount() int { return len(fm.rows) }

func (fm *FormModal) SetRowHidden(rowIdx int, hidden bool) {
	if rowIdx >= 0 && rowIdx < len(fm.rows) {
		fm.rows[rowIdx].hidden = hidden
	}
}

// BeginSection starts a page: rows added after it are laid out only while that section is open.
func (fm *FormModal) BeginSection(name string) {
	fm.pickerRow = nil
	if fm.sectionRail == nil {
		fm.buildSectionRail()
	}
	fm.sections = append(fm.sections, formSection{name: name})
}

func (fm *FormModal) buildSectionRail() {
	theme := fm.app.theme

	fm.sectionRail = tview.NewTextView()
	fm.sectionRail.SetDynamicColors(true)
	fm.sectionRail.SetWrap(false)
	fm.sectionRail.SetBackgroundColor(theme.ModalBackground())
	fm.railRule = fm.app.modalColumnRule(func() int {
		if fm.contextText != "" {
			return -1
		}
		_, top, _, _ := fm.frame.GetInnerRect()
		return top
	})
	fm.footerRule = fm.formFooterRule()
	fm.navColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.navColumn.SetBackgroundColor(theme.ModalBackground())
	fm.buttonsColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	fm.buttonsColumn.SetBackgroundColor(theme.ModalBackground())

	fm.registerFocusable(fm.sectionRail, -1)
	fm.sectionRail.SetFocusFunc(func() {
		fm.railFocused = true
		fm.onFocused(fm.sectionRail, -1)
	})
	fm.sectionRail.SetBlurFunc(func() { fm.railFocused = false })
	fm.sectionRail.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action != tview.MouseLeftDown || !fm.railIsVertical() {
			return action, event
		}
		x, y := event.Position()
		if !fm.sectionRail.InRect(x, y) {
			return action, event
		}
		_, railY, _, _ := fm.sectionRail.GetInnerRect()
		if index := y - railY - railTopPad; index >= 0 && index < len(fm.sections) {
			fm.SetActiveSection(index)
		}
		return action, event
	})
}

// The junction column is computed, not read off the rule: a Flex defers a focused child's draw.
func (fm *FormModal) formFooterRule() *tview.Box {
	rule := tview.NewBox()
	rule.SetBackgroundColor(fm.app.theme.ModalBackground())
	rule.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		style := tcell.StyleDefault.
			Background(fm.app.theme.ModalBackground()).
			Foreground(fm.app.theme.BorderFocus)
		padding := fm.app.density.ModalPadding
		left, right := x-padding.Left-1, x+width+padding.Right
		joint := -1
		if fm.railIsVertical() {
			joint = x + fm.railColumn() + fm.railGutter()
		}
		screen.SetContent(left, y, tview.Borders.LeftT, nil, style)
		for col := left + 1; col < right; col++ {
			glyph := tview.Borders.Horizontal
			if col == joint {
				glyph = tview.Borders.BottomT
			}
			screen.SetContent(col, y, glyph, nil, style)
		}
		screen.SetContent(right, y, tview.Borders.RightT, nil, style)
		return x, y, width, height
	})
	return rule
}

func (fm *FormModal) buildButtonsColumn() {
	fm.buttonsColumn.Clear()
	for i, btn := range fm.buttons {
		if i > 0 {
			fm.buttonsColumn.AddItem(nil, sectionRailSpacerRows, 0, false)
		}
		fm.buttonsColumn.AddItem(btn, 1, 0, false)
	}
}

func (fm *FormModal) buttonsColumnRows() int {
	if len(fm.buttons) == 0 {
		return 0
	}
	return len(fm.buttons) + (len(fm.buttons)-1)*sectionRailSpacerRows
}

func (fm *FormModal) SetActiveSection(index int) {
	if index < 0 || index >= len(fm.sections) || index == fm.activeSection {
		return
	}
	fm.activeSection = index
	fm.scrollTop = 0
	fm.layout()
	if p := fm.focusedPrimitive(); p != nil && !fm.focusReachable(p) {
		fm.focusRail()
	}
}

func (fm *FormModal) focusRail() {
	if fm.sectionRail == nil {
		return
	}
	for i, candidate := range fm.order {
		if candidate == fm.sectionRail {
			fm.focusIdx = i
			break
		}
	}
	fm.app.app.SetFocus(fm.sectionRail)
}

func (fm *FormModal) enterSection() {
	for i, candidate := range fm.order {
		if candidate == fm.sectionRail || !fm.focusReachable(candidate) {
			continue
		}
		if rowIdx, ok := fm.rowOf[candidate]; !ok || rowIdx < 0 {
			continue
		}
		fm.focusIdx = i
		fm.app.app.SetFocus(candidate)
		return
	}
}

func (fm *FormModal) railHasFocus() bool {
	return fm.sectionRail != nil && fm.focusedPrimitive() == fm.sectionRail
}

func (fm *FormModal) rowVisible(row formRow) bool {
	return fm.rowVisibleIn(row, fm.activeSection)
}

func (fm *FormModal) rowVisibleIn(row formRow, section int) bool {
	if row.hidden {
		return false
	}
	return len(fm.sections) == 0 || row.section < 0 || row.section == section
}

func (fm *FormModal) railGutter() int {
	return fm.app.density.ModalPadding.Left
}

func (fm *FormModal) railGap() int {
	return fm.railGutter()*2 + 1
}

func (fm *FormModal) railColumn() int {
	return fm.railWidth() - fm.railGap()
}

func (fm *FormModal) railWidth() int {
	widest := 0
	for _, section := range fm.sections {
		if length := len([]rune(section.name)); length > widest {
			widest = length
		}
	}
	return widest + sectionRailPad + fm.railGap()
}

func (fm *FormModal) railIsVertical() bool {
	if len(fm.sections) < 2 {
		return false
	}
	return fm.innerWidth()-fm.railWidth() >= formRowsMinWidth
}

// Called from the frame's draw func, never a focus callback: TextView.MouseHandler holds the view's lock while moving focus.
func (fm *FormModal) renderSectionRail() {
	if fm.sectionRail == nil {
		return
	}
	tags := fm.app.themeTags
	if !fm.railIsVertical() {
		name := ""
		if fm.activeSection < len(fm.sections) {
			name = fm.sections[fm.activeSection].name
		}
		fm.sectionRail.SetText(tags.Accent + "‹ " + name + " ›[-:-:-]")
		return
	}

	width := fm.railColumn()
	lines := make([]string, 0, len(fm.sections)+railTopPad)
	for range railTopPad {
		lines = append(lines, "")
	}
	for i, section := range fm.sections {
		if i != fm.activeSection {
			lines = append(lines, tags.SecondaryText+section.name+"[-:-:-]")
			continue
		}
		if !fm.railFocused {
			lines = append(lines, tags.Accent+section.name+"[-:-:-]")
			continue
		}
		pad := width - len([]rune(section.name))
		if pad < 0 {
			pad = 0
		}
		lines = append(lines, tags.Selection+section.name+strings.Repeat(" ", pad)+"[-:-:-]")
	}
	fm.sectionRail.SetText(strings.Join(lines, "\n"))
}

func (fm *FormModal) stepSection(delta int) {
	if len(fm.sections) == 0 {
		return
	}
	fm.SetActiveSection((fm.activeSection + delta + len(fm.sections)) % len(fm.sections))
}

func (fm *FormModal) sidebarStops() int {
	return len(fm.sections) + len(fm.buttons)
}

func (fm *FormModal) focusedButton() int {
	focused := fm.focusedPrimitive()
	for i, btn := range fm.buttons {
		if btn == focused {
			return i
		}
	}
	return -1
}

func (fm *FormModal) focusButton(index int) {
	if index < 0 || index >= len(fm.buttons) {
		return
	}
	target := fm.buttons[index]
	for i, candidate := range fm.order {
		if candidate == target {
			fm.focusIdx = i
			break
		}
	}
	fm.app.app.SetFocus(target)
}

func (fm *FormModal) stepSidebar(delta int) {
	stops := fm.sidebarStops()
	if stops == 0 {
		return
	}
	at := fm.activeSection
	if button := fm.focusedButton(); button >= 0 {
		at = len(fm.sections) + button
	}
	next := (at + delta + stops) % stops
	if next < len(fm.sections) {
		fm.SetActiveSection(next)
		fm.focusRail()
		return
	}
	fm.focusButton(next - len(fm.sections))
}

// EndRow closes the packed row AddPicker and AddPackedInput are filling, so the next one starts a new row.
func (fm *FormModal) EndRow() { fm.pickerRow = nil }

func (fm *FormModal) SetOnCancel(fn func()) { fm.onCancel = fn }

func (fm *FormModal) SetOnSubmit(fn func()) { fm.onSubmit = fn }

func (fm *FormModal) newInput(initial string) *tview.InputField {
	input := tview.NewInputField().
		SetFieldBackgroundColor(fm.app.theme.ModalBackground()).
		SetFieldTextColor(fm.app.theme.Foreground).
		SetFieldWidth(0).
		SetText(initial)
	input.SetBackgroundColor(fm.app.theme.ModalBackground())
	return input
}

func (fm *FormModal) AddInput(label, initial string) *tview.InputField {
	input := fm.newInput(initial)
	fm.addFramedRow(label, input, 1, false)
	return input
}

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

func (fm *FormModal) AddPackedInput(label, initial string) *tview.InputField {
	input := fm.newInput(initial)
	rowIdx := fm.packField(label, input)
	fm.registerFocusable(input, rowIdx)
	return input
}

func (fm *FormModal) packField(label string, field tview.Primitive) int {
	theme := fm.app.theme

	labelView := fm.capsLabel(label)

	frame := tview.NewFlex().SetDirection(tview.FlexRow)
	frame.Box = tview.NewBox()
	frame.AddItem(field, 0, 1, true)
	frame.SetBackgroundColor(theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(theme.Border)

	if fm.pickerRow == nil {
		container := tview.NewFlex().SetDirection(tview.FlexRow)
		container.SetBackgroundColor(theme.ModalBackground())

		fm.appendRow(formRow{
			container: container,
			height:    formFieldRows,
			minHeight: formFieldRows,
		})
		fm.pickerRow = &pickerRowState{rowIdx: len(fm.rows) - 1}
	}

	rowIdx := fm.pickerRow.rowIdx
	row := &fm.rows[rowIdx]
	row.packed = append(row.packed, packedColumn{
		labelView: labelView,
		frame:     frame,
		labelLen:  len([]rune(strings.ToUpper(label))),
	})
	row.focusables = append(row.focusables, field)
	row.columns++
	fm.frameOf[field] = frame
	return rowIdx
}

func packedLines(row *formRow, innerWidth int) (perLine int) {
	count := len(row.packed)
	if count <= 1 || innerWidth <= 0 {
		return 1
	}
	widest := 0
	for _, column := range row.packed {
		if column.labelLen > widest {
			widest = column.labelLen
		}
	}
	if widest > packedLabelBudget {
		widest = packedLabelBudget
	}
	for _, candidate := range []int{count, 2, 1} {
		if candidate > count {
			continue
		}
		share := (innerWidth - packedColumnGap*(candidate-1)) / candidate
		if share >= widest {
			return candidate
		}
	}
	return 1
}

func (fm *FormModal) relayoutPacked(row *formRow, innerWidth int) {
	if len(row.packed) == 0 {
		return
	}
	perLine := packedLines(row, innerWidth)
	if row.perLine == perLine && row.container.GetItemCount() > 0 {
		return
	}
	row.perLine = perLine

	theme := fm.app.theme
	row.container.Clear()
	for start := 0; start < len(row.packed); start += perLine {
		end := start + perLine
		if end > len(row.packed) {
			end = len(row.packed)
		}
		labels := tview.NewFlex()
		labels.SetBackgroundColor(theme.ModalBackground())
		values := tview.NewFlex()
		values.SetBackgroundColor(theme.ModalBackground())
		for i, column := range row.packed[start:end] {
			if i > 0 {
				labels.AddItem(nil, packedColumnGap, 0, false)
				values.AddItem(nil, packedColumnGap, 0, false)
			}
			labels.AddItem(column.labelView, 0, 1, false)
			values.AddItem(column.frame, 0, 1, true)
		}
		row.container.AddItem(labels, 1, 0, false)
		row.container.AddItem(values, 3, 0, start == 0)
	}
	lines := (len(row.packed) + perLine - 1) / perLine
	row.height = formFieldRows * lines
	row.minHeight = row.height
}

// Wrapping is off because the label is mounted one line tall, where a wrapped label draws only its first word.
func (fm *FormModal) capsLabel(label string) *tview.TextView {
	view := tview.NewTextView()
	view.SetWrap(false)
	view.SetText(strings.ToUpper(label))
	view.SetTextColor(fm.app.theme.SecondaryText)
	view.SetBackgroundColor(fm.app.theme.ModalBackground())
	return view
}

func (fm *FormModal) AddCheckbox(label string, checked bool) *tview.Checkbox {
	theme := fm.app.theme

	box := tview.NewCheckbox().SetChecked(checked)
	fieldStyle := tcell.StyleDefault.Background(theme.InputBg).Foreground(theme.Foreground)
	box.SetUncheckedStyle(fieldStyle)
	box.SetCheckedStyle(fieldStyle)
	box.SetActivatedStyle(tcell.StyleDefault.
		Background(theme.Accent).
		Foreground(theme.InverseTextColor()))
	box.SetBackgroundColor(theme.ModalBackground())

	caps := strings.ToUpper(label)
	labelView := fm.capsLabel(label)

	line := tview.NewFlex()
	line.SetBackgroundColor(theme.ModalBackground())
	line.AddItem(labelView, len(caps)+2, 0, false)
	line.AddItem(box, 3, 0, true)
	line.AddItem(nil, 0, 1, false)

	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBackgroundColor(theme.ModalBackground())
	container.AddItem(nil, 1, 0, false)
	container.AddItem(line, 1, 0, true)
	container.AddItem(nil, 1, 0, false)

	fm.pickerRow = nil
	fm.appendRow(formRow{
		container:  container,
		height:     3,
		minHeight:  3,
		focusables: []tview.Primitive{box},
		labelView:  labelView,
	})
	fm.checkboxLabels[box] = labelView
	fm.registerFocusable(box, len(fm.rows)-1)
	return box
}

func (fm *FormModal) AddStatic(text string) *tview.TextView {
	view := tview.NewTextView()
	view.SetText(text)
	view.SetTextColor(fm.app.theme.SecondaryText)
	view.SetBackgroundColor(fm.app.theme.ModalBackground())
	fm.pickerRow = nil
	fm.appendRow(formRow{container: staticRowContainer(view, fm.app.theme), height: 1, minHeight: 1})
	return view
}

func staticRowContainer(view *tview.TextView, theme Theme) *tview.Flex {
	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBackgroundColor(theme.ModalBackground())
	container.AddItem(view, 1, 0, false)
	return container
}

func (fm *FormModal) fieldUnit(label string, editor tview.Primitive) (container *tview.Flex, labelView *tview.TextView) {
	theme := fm.app.theme

	labelView = fm.capsLabel(label)

	frame := tview.NewFlex().SetDirection(tview.FlexRow)
	frame.Box = tview.NewBox()
	frame.AddItem(editor, 0, 1, true)
	frame.SetBackgroundColor(theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(theme.Border)

	container = tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBackgroundColor(theme.ModalBackground())
	container.AddItem(labelView, 1, 0, false)
	container.AddItem(frame, 0, 1, true)

	fm.frameOf[editor] = frame
	return container, labelView
}

func (fm *FormModal) addFramedRow(label string, editor tview.Primitive, editorRows int, flexible bool) {
	fm.pickerRow = nil
	container, labelView := fm.fieldUnit(label, editor)

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
	fm.registerFocusable(editor, len(fm.rows)-1)
}

func (fm *FormModal) AddSplitRow(label string, rows int, sideLabels []string) (*FormMultiSelect, []*tview.InputField) {
	fm.pickerRow = nil
	theme := fm.app.theme

	multi := fm.newMultiSelect()
	listContainer, labelView := fm.fieldUnit(label, multi.list)

	side := tview.NewFlex().SetDirection(tview.FlexRow)
	side.SetBackgroundColor(theme.ModalBackground())
	inputs := make([]*tview.InputField, 0, len(sideLabels))
	for _, sideLabel := range sideLabels {
		input := fm.newInput("")
		container, _ := fm.fieldUnit(sideLabel, input)
		side.AddItem(container, formFieldRows, 0, true)
		inputs = append(inputs, input)
	}

	columns := tview.NewFlex()
	columns.SetBackgroundColor(theme.ModalBackground())
	columns.AddItem(listContainer, 0, 1, true)
	columns.AddItem(nil, 2, 0, false)
	columns.AddItem(side, 0, 1, false)

	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBackgroundColor(theme.ModalBackground())
	container.AddItem(columns, 0, 1, true)

	height := 1 + rows + 2
	stack := formFieldRows * len(sideLabels)
	if stack > height {
		height = stack
	}
	focusables := []tview.Primitive{multi.list}
	for _, input := range inputs {
		focusables = append(focusables, input)
	}
	fm.appendRow(formRow{
		container:  container,
		height:     height,
		minHeight:  stack,
		flexible:   height > stack,
		focusables: focusables,
		labelView:  labelView,
	})
	for _, focusable := range focusables {
		fm.registerFocusable(focusable, len(fm.rows)-1)
	}
	return multi, inputs
}

func (fm *FormModal) SetPlaceholder(input *tview.InputField, text string) {
	input.SetPlaceholder(text)
	input.SetPlaceholderStyle(tcell.StyleDefault.
		Background(fm.app.theme.ModalBackground()).
		Foreground(fm.app.theme.SecondaryText))
}

func (fm *FormModal) appendRow(row formRow) {
	row.section = len(fm.sections) - 1
	fm.rows = append(fm.rows, row)
}

func (fm *FormModal) registerFocusable(p tview.Primitive, rowIdx int) {
	fm.order = append(fm.order, p)
	fm.rowOf[p] = rowIdx
	if box, ok := p.(interface{ SetFocusFunc(func()) *tview.Box }); ok {
		box.SetFocusFunc(func() { fm.onFocused(p, rowIdx) })
	}
}

func (fm *FormModal) onFocused(p tview.Primitive, rowIdx int) {
	if picker := fm.openPicker; picker != nil && picker.view != p {
		picker.closeMenu()
	}
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
	for box, label := range fm.checkboxLabels {
		if box == p {
			label.SetTextColor(fm.app.theme.Foreground)
		} else {
			label.SetTextColor(fm.app.theme.SecondaryText)
		}
	}
	if rowIdx >= 0 {
		fm.ensureVisible(rowIdx)
	}
}

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
		fm.buttonLabels = append(fm.buttonLabels, spec.Label)
		fm.registerFocusable(btn, -1)
	}
	fm.buttonsRow.AddItem(nil, 0, 1, false)
}

func (fm *FormModal) effectiveMaxWidth() int {
	if fm.maxWidth > 0 {
		return fm.maxWidth
	}
	return formModalDefaultMaxWidth
}

func (fm *FormModal) chromeHeight() int {
	if fm.railIsVertical() {
		chrome := 2 + 1 + 1
		if fm.contextText != "" {
			chrome += 2
		}
		return chrome
	}
	chrome := 2 + 1 + 1
	if fm.buttonsRow != nil {
		chrome += 2
	}
	if fm.contextText != "" {
		chrome += 2
	}
	if len(fm.sections) > 1 && !fm.railIsVertical() {
		chrome += 2
	}
	return chrome
}

func (fm *FormModal) panelWidth() int {
	screenW, _ := fm.screenSize()
	width := screenW - formModalScreenWMargin
	if limit := fm.effectiveMaxWidth(); width > limit || width <= 0 {
		width = limit
	}
	return width
}

func (fm *FormModal) innerWidth() int {
	padding := fm.app.density.ModalPadding
	return fm.panelWidth() - 2 - padding.Left - padding.Right
}

func (fm *FormModal) rowsWidth() int {
	if fm.railIsVertical() {
		return fm.innerWidth() - fm.railWidth()
	}
	return fm.innerWidth()
}

func (fm *FormModal) rowHeights(screenH int) []int {
	inner := fm.rowsWidth()
	for i := range fm.rows {
		fm.relayoutPacked(&fm.rows[i], inner)
	}
	heights := make([]int, len(fm.rows))
	total := fm.chromeHeight()
	for i, row := range fm.rows {
		if !fm.rowVisible(row) {
			continue
		}
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

func (fm *FormModal) contentHeight(screenH int) int {
	total := fm.chromeHeight() + fm.tallestSectionRows(screenH)
	if maxHeight := screenH - formModalScreenHMargin; total > maxHeight {
		return maxHeight
	}
	return total
}

func (fm *FormModal) tallestSectionRows(screenH int) int {
	heights := fm.rowHeights(screenH)
	if len(fm.sections) == 0 {
		total := 0
		for _, h := range heights {
			total += h
		}
		return total
	}

	tallest := 0
	if fm.railIsVertical() {
		tallest = railTopPad + len(fm.sections) + fm.buttonsColumnRows()
	}
	for section := range fm.sections {
		total := 0
		for i, row := range fm.rows {
			if !fm.rowVisibleIn(row, section) {
				continue
			}
			if heights[i] > 0 && heights[i] < row.height {
				total += heights[i]
				continue
			}
			total += row.height
		}
		if total > tallest {
			tallest = total
		}
	}
	return tallest
}

func (fm *FormModal) screenSize() (int, int) {
	_, _, w, h := fm.app.pages.GetRect()
	return w, h
}

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

// A Flex hands every fixed-size child its full size, so a row outside the window is unmounted rather than sized to zero.
func (fm *FormModal) applyRowWindow(heights []int, avail int) []int {
	shown := make([]int, len(fm.rows))
	used := 0
	clipped := false
	fm.rowsBox.Clear()
	for i, row := range fm.rows {
		if i < fm.scrollTop {
			continue
		}
		h := 0
		if remaining := avail - used; remaining > 0 {
			h = heights[i]
			if h > remaining {
				h = remaining
			}
			used += h
		}
		if h < heights[i] {
			clipped = true
		}
		shown[i] = h
		if h > 0 {
			fm.rowsBox.AddItem(row.container, h, 0, len(row.focusables) > 0)
		}
	}
	fm.scrollAbove = fm.scrollTop > 0
	fm.scrollBelow = clipped
	return shown
}

func (fm *FormModal) layout() {
	centerModal(fm.root, fm.frame, func() (int, int) {
		_, screenH := fm.screenSize()
		fm.layoutFrame()
		fm.layoutBody()
		height := fm.contentHeight(screenH)
		fm.applyRowWindow(fm.rowHeights(screenH), height-fm.chromeHeight())
		return fm.panelWidth(), height
	})
}

func (fm *FormModal) layoutFrame() {
	fm.frame.Clear()
	if fm.contextText != "" {
		fm.frame.AddItem(fm.contextView, 1, 0, false)
		fm.frame.AddItem(nil, 1, 0, false)
	}
	fm.frame.AddItem(fm.body, 0, 1, true)
	if fm.railIsVertical() {
		fm.frame.AddItem(fm.footerRule, 1, 0, false)
		fm.frame.AddItem(fm.hintView, 1, 0, false)
		return
	}
	if fm.buttonsRow != nil {
		fm.frame.AddItem(nil, 1, 0, false)
		fm.frame.AddItem(fm.buttonsRow, 1, 0, false)
	}
	fm.frame.AddItem(nil, 1, 0, false)
	fm.frame.AddItem(fm.hintView, 1, 0, false)
}

func (fm *FormModal) layoutBody() {
	fm.body.Clear()
	if len(fm.sections) < 2 {
		fm.body.SetDirection(tview.FlexRow).AddItem(fm.rowsBox, 0, 1, true)
		return
	}
	if fm.railIsVertical() {
		fm.buildButtonsColumn()
		fm.navColumn.Clear()
		fm.navColumn.AddItem(fm.sectionRail, 0, 1, true)
		if rows := fm.buttonsColumnRows(); rows > 0 {
			fm.navColumn.AddItem(fm.buttonsColumn, rows, 0, false)
		}
		fm.body.SetDirection(tview.FlexColumn).
			AddItem(fm.navColumn, fm.railColumn(), 0, true).
			AddItem(nil, fm.railGutter(), 0, false).
			AddItem(fm.railRule, 1, 0, false).
			AddItem(nil, fm.railGutter(), 0, false).
			AddItem(fm.rowsBox, 0, 1, false)
		return
	}
	fm.body.SetDirection(tview.FlexRow).
		AddItem(fm.sectionRail, 1, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(fm.rowsBox, 0, 1, false)
}

func (fm *FormModal) Show(pageName string) {
	fm.scrollTop = 0
	fm.focusIdx = fm.initialFocusIdx()
	fm.openPicker = nil
	fm.SetStatus("", false)
	fm.layout()
	if len(fm.order) > 0 && !fm.focusReachable(fm.order[fm.focusIdx]) {
		for i, candidate := range fm.order {
			if fm.focusReachable(candidate) {
				fm.focusIdx = i
				break
			}
		}
	}
	fm.app.pages.AddPage(pageName, fm.page, true, true)
	fm.app.pages.SendToFront(pageName)
	if len(fm.order) > 0 {
		fm.app.app.SetFocus(fm.order[fm.focusIdx])
	}
}

func (fm *FormModal) initialFocusIdx() int {
	for i, candidate := range fm.order {
		if candidate == fm.initialFocus {
			return i
		}
	}
	return 0
}

func (fm *FormModal) Hide(pageName string) {
	if fm.openPicker != nil {
		fm.openPicker.closeMenu()
	}
	fm.app.pages.RemovePage(pageName)
	fm.app.restoreModalFocus()
}

func (fm *FormModal) Root() *tview.Flex { return fm.root }

// ContentBody mounts the rows at full height for an embedding modal that owns the shell and never calls Show. Call once, after the fields.
func (fm *FormModal) ContentBody() *tview.Flex {
	fm.rowsBox.Clear()
	for i := range fm.rows {
		row := &fm.rows[i]
		fm.relayoutPacked(row, fm.rowsWidth())
		if !fm.rowVisible(*row) {
			continue
		}
		fm.rowsBox.AddItem(row.container, row.height, 0, len(row.focusables) > 0)
	}

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(fm.app.theme.ModalBackground())
	body.AddItem(fm.rowsBox, 0, 1, true)
	return body
}

// ButtonsRow returns the centered button row, nil until AddButtons is called.
func (fm *FormModal) ButtonsRow() *tview.Flex { return fm.buttonsRow }

func (fm *FormModal) SetInitialFocus(p tview.Primitive) { fm.initialFocus = p }

func (fm *FormModal) Focus() {
	if p := fm.focusedPrimitive(); p != nil {
		fm.app.app.SetFocus(p)
	}
}

func (fm *FormModal) BlurFrames() {
	for _, frame := range fm.frameOf {
		frame.SetBorderColor(fm.app.theme.Border)
	}
}

func (fm *FormModal) focusReachable(p tview.Primitive) bool {
	if p == fm.sectionRail {
		return len(fm.sections) > 1
	}
	rowIdx, ok := fm.rowOf[p]
	if !ok || rowIdx < 0 || rowIdx >= len(fm.rows) {
		return true
	}
	return fm.rowVisible(fm.rows[rowIdx])
}

func (fm *FormModal) focusStep(delta int) {
	if len(fm.order) == 0 {
		return
	}
	for range fm.order {
		fm.focusIdx = (fm.focusIdx + delta + len(fm.order)) % len(fm.order)
		if fm.focusReachable(fm.order[fm.focusIdx]) {
			break
		}
	}
	fm.app.app.SetFocus(fm.order[fm.focusIdx])
}

// HandleKey is called from the modal dispatcher, since a parent's InputCapture never sees keys sent to a focused child.
func (fm *FormModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if fm.openPicker != nil {
		return fm.handleMenuKey(event)
	}

	if fm.railIsVertical() && (fm.railHasFocus() || fm.focusedButton() >= 0) {
		switch {
		case event.Key() == tcell.KeyUp, event.Key() == tcell.KeyRune && event.Rune() == 'k':
			fm.stepSidebar(-1)
			return nil
		case event.Key() == tcell.KeyDown, event.Key() == tcell.KeyRune && event.Rune() == 'j':
			fm.stepSidebar(1)
			return nil
		case fm.railHasFocus() && (event.Key() == tcell.KeyEnter ||
			event.Key() == tcell.KeyRight ||
			event.Key() == tcell.KeyRune && event.Rune() == 'l'):
			fm.enterSection()
			return nil
		}
	}
	if fm.railHasFocus() {
		switch {
		case event.Key() == tcell.KeyUp, event.Key() == tcell.KeyRune && event.Rune() == 'k':
			fm.stepSection(-1)
			return nil
		case event.Key() == tcell.KeyDown, event.Key() == tcell.KeyRune && event.Rune() == 'j':
			fm.stepSection(1)
			return nil
		case event.Key() == tcell.KeyEnter,
			event.Key() == tcell.KeyRight,
			event.Key() == tcell.KeyRune && event.Rune() == 'l':
			fm.enterSection()
			return nil
		}
	}

	switch event.Key() {
	case tcell.KeyEscape:
		if len(fm.sections) > 1 && !fm.railHasFocus() {
			fm.focusRail()
			return nil
		}
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
	case tcell.KeyRune:
		if ms := fm.focusedMultiSelect(); ms != nil && (event.Rune() == ' ' || event.Rune() == 't') {
			ms.toggle()
			return nil
		}
	case tcell.KeyEnter:
		if event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			if fm.onSubmit != nil {
				fm.onSubmit()
			}
			return nil
		}
		if ms := fm.focusedMultiSelect(); ms != nil {
			ms.toggle()
			return nil
		}
		if picker := fm.focusedPicker(); picker != nil {
			picker.openMenu()
			return nil
		}
		if _, ok := fm.focusedPrimitive().(*tview.InputField); ok {
			fm.focusStep(1)
			return nil
		}
	}
	return event
}

func (fm *FormModal) focusedMultiSelect() *FormMultiSelect {
	list, ok := fm.focusedPrimitive().(*tview.List)
	if !ok {
		return nil
	}
	return fm.multiSelects[list]
}

func (fm *FormModal) focusedPrimitive() tview.Primitive {
	if fm.focusIdx >= 0 && fm.focusIdx < len(fm.order) {
		return fm.order[fm.focusIdx]
	}
	return nil
}
