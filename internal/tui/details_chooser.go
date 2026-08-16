package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

const (
	// detailsChooserMaxRows is the most options a chooser draws before the rest
	// go behind a count.
	detailsChooserMaxRows = 10
	// detailsChooserChrome is the rows a chooser spends on itself: the two
	// frame edges and the row counting what it did not draw.
	detailsChooserChrome = 3
)

// chooserClearLabels name the empty value of each field that has one, in the
// words read mode uses for it.
var chooserClearLabels = map[issueField]string{
	issueFieldAssignee:  "Unassigned",
	issueFieldProject:   "No project",
	issueFieldMilestone: "No milestone",
	issueFieldCycle:     "No cycle",
}

// fieldHasChooser is the fields Enter opens. The rest still edit through their
// own commands.
func fieldHasChooser(field issueField) bool {
	switch field {
	case issueFieldState, issueFieldAssignee, issueFieldPriority,
		issueFieldProject, issueFieldMilestone, issueFieldCycle:
		return true
	}
	return false
}

// openFieldChooser drops the cursor's field options into the page under it.
func (a *App) openFieldChooser() {
	if !a.detailsEdit.on || a.detailsEdit.open != "" || !fieldHasChooser(a.detailsEdit.cursor) {
		return
	}
	issue := a.GetSelectedIssue()
	// The page is what the options are about, and inside the detail debounce the
	// selection has already moved off it.
	if issue == nil || issue.ID != a.detailsIssueID {
		return
	}
	field := a.detailsEdit.cursor
	gen := a.chooserGeneration.Add(1)
	// Written before the loader runs: a warm cache answers inside this call, and
	// a flag set after it would strand the chooser on its loading row.
	a.detailsEdit.open = field
	a.detailsEdit.issue = *issue
	a.detailsEdit.options = nil
	a.detailsEdit.choice, a.detailsEdit.offset = 0, 0
	a.detailsEdit.loading = true
	a.detailsEdit.gen = gen
	a.renderDetailsPage()
	a.updateStatusBar()

	a.issueFieldOptions(field, a.issueOptionScope(*issue),
		func(items []PickerItem) { a.fillFieldChooser(gen, field, items) },
		func(err error) { a.failFieldChooser(gen, field, err) })
}

// chooserIsCurrent reports whether a load still belongs to the chooser on the
// page. The generation lives on App so a mode exit cannot restart it at zero.
func (a *App) chooserIsCurrent(gen uint64, field issueField) bool {
	edit := a.detailsEdit
	return edit.on && edit.open == field && edit.gen == gen && edit.issue.ID == a.detailsIssueID
}

// fillFieldChooser puts the loaded options on the page, opening the highlight on
// the value the field already holds.
func (a *App) fillFieldChooser(gen uint64, field issueField, items []PickerItem) {
	if !a.chooserIsCurrent(gen, field) {
		return
	}
	current := currentFieldOptionID(field, a.detailsEdit.issue)
	// No clear row on a field that is already empty, or Enter is a key that
	// does nothing visible.
	if label, ok := chooserClearLabels[field]; ok && current != "" {
		items = append([]PickerItem{{Label: label}}, items...)
	}
	a.detailsEdit.loading = false
	a.detailsEdit.options = items
	a.detailsEdit.choice = chooserIndexOf(items, current)
	a.detailsEdit.offset = max(0, a.detailsEdit.choice-a.chooserVisibleRows()+1)
	a.renderDetailsPage()
	a.scrollChooserIntoView()
}

// failFieldChooser closes a chooser whose options never arrived, rather than
// leaving it on a loading row no key can clear.
func (a *App) failFieldChooser(gen uint64, field issueField, err error) {
	if !a.chooserIsCurrent(gen, field) {
		return
	}
	a.closeFieldChooser()
	a.updateStatusBarWithError(err)
}

// closeFieldChooser takes the options off the page and leaves edit mode on.
func (a *App) closeFieldChooser() {
	if a.detailsEdit.open == "" {
		return
	}
	a.detailsEdit.open = ""
	a.detailsEdit.issue = linearapi.Issue{}
	a.detailsEdit.options = nil
	a.detailsEdit.choice, a.detailsEdit.offset = 0, 0
	a.detailsEdit.loading = false
	a.renderDetailsPage()
	// The rows it held are gone and everything below has moved up, which can put
	// the field it belongs to off the top.
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

// commitFieldChooser saves the highlighted option and closes.
func (a *App) commitFieldChooser() {
	edit := a.detailsEdit
	if edit.open == "" || edit.loading || edit.choice < 0 || edit.choice >= len(edit.options) {
		return
	}
	field, item, opened := edit.open, edit.options[edit.choice], edit.issue
	issue := a.chooserIssue()
	a.closeFieldChooser()
	if item.ID == currentFieldOptionID(field, issue) {
		return
	}
	if moved := chooserScopeMoved(field, opened, issue); moved != "" {
		a.flashError("The " + moved + " changed, pick again")
		return
	}
	if save, ok := chooserSave(field, issue, item); ok {
		a.saveIssueField(save)
	}
}

// chooserScopeMoved names the scope the options no longer belong to, empty
// while they still do. A refresh that moved the issue makes every id refusable.
func chooserScopeMoved(field issueField, opened, now linearapi.Issue) string {
	switch field {
	case issueFieldPriority:
		// A local list, scoped to nothing.
		return ""
	case issueFieldMilestone:
		if now.ProjectID != opened.ProjectID {
			return "project"
		}
	default:
		if now.TeamID != opened.TeamID {
			return "team"
		}
	}
	return ""
}

// chooserIssue is the issue the chooser opened on, refreshed from the selection
// while it is still that one: the id is the write target, the rest is state.
func (a *App) chooserIssue() linearapi.Issue {
	opened := a.detailsEdit.issue
	a.issuesMu.RLock()
	selected := a.selectedIssue
	a.issuesMu.RUnlock()
	if selected != nil && selected.ID == opened.ID {
		return *selected
	}
	return opened
}

// chooserSave builds one field's write from the option picked. An empty id is
// the clear row.
func chooserSave(field issueField, issue linearapi.Issue, item PickerItem) (issueFieldSave, bool) {
	switch field {
	case issueFieldState:
		return issueFieldStateSave(issue, item.ID, item.name()), true
	case issueFieldPriority:
		priority, err := strconv.Atoi(item.ID)
		if err != nil {
			return issueFieldSave{}, false
		}
		return issueFieldPrioritySave(issue, priority), true
	case issueFieldAssignee:
		if item.ID == "" {
			return issueFieldAssigneeClear(issue), true
		}
		return issueFieldAssigneeSave(issue, item.ID, item.name()), true
	case issueFieldProject:
		if item.ID == "" {
			return issueFieldProjectClear(issue), true
		}
		return issueFieldProjectSave(issue, item.ID, item.name()), true
	case issueFieldMilestone:
		if item.ID == "" {
			return issueFieldMilestoneClear(issue), true
		}
		return issueFieldMilestoneSave(issue, item.ID, item.name()), true
	case issueFieldCycle:
		if item.ID == "" {
			return issueFieldCycleClear(issue), true
		}
		return issueFieldCycleSave(issue, item.ID, item.name()), true
	}
	return issueFieldSave{}, false
}

// currentFieldOptionID is the option id the field holds now, empty when it
// holds none.
func currentFieldOptionID(field issueField, issue linearapi.Issue) string {
	switch field {
	case issueFieldState:
		return issue.StateID
	case issueFieldAssignee:
		return issue.AssigneeID
	case issueFieldPriority:
		return strconv.Itoa(issue.Priority)
	case issueFieldProject:
		return issue.ProjectID
	case issueFieldMilestone:
		if issue.ProjectMilestone != nil {
			return issue.ProjectMilestone.ID
		}
	case issueFieldCycle:
		if issue.Cycle != nil {
			return issue.Cycle.ID
		}
	}
	return ""
}

// chooserIndexOf finds the option a field already holds, and -1 for a value the
// list does not carry, which lights no row rather than lying about the first.
func chooserIndexOf(items []PickerItem, id string) int {
	if id == "" {
		return 0
	}
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

// moveChooserChoice steps the highlight one option down (+1) or up (-1),
// stopping at both ends the way the field cursor does.
func (a *App) moveChooserChoice(step int) {
	if a.detailsEdit.open == "" || len(a.detailsEdit.options) == 0 {
		return
	}
	next := a.detailsEdit.choice + step
	if next < 0 || next >= len(a.detailsEdit.options) {
		return
	}
	a.detailsEdit.choice = next
	visible := a.chooserVisibleRows()
	if next < a.detailsEdit.offset {
		a.detailsEdit.offset = next
	} else if next >= a.detailsEdit.offset+visible {
		a.detailsEdit.offset = next - visible + 1
	}
	a.renderDetailsPage()
	a.scrollChooserIntoView()
}

// scrollChooserIntoView brings the highlighted option onto the page. The row,
// not the block: a block taller than the pane anchors to its top.
func (a *App) scrollChooserIntoView() {
	if a.detailsEdit.open == "" || a.detailsChooserRow < 0 {
		return
	}
	a.scrollRowsIntoView(a.detailsChooserRow, a.detailsChooserRow)
}

// chooserVisibleRows is how many options fit: the cap, or fewer on a pane too
// short to reach the bottom of one.
func (a *App) chooserVisibleRows() int {
	rows := detailsChooserMaxRows
	// The height the last draw handed the refit, not the view's own rect, which
	// during a draw is still the frame before this one.
	if height := a.detailsFittedHeight; height > 0 {
		rows = min(rows, max(1, height-detailsChooserChrome))
	}
	return rows
}

// chooserWindow is the half-open range of options drawn, clamped here because
// this runs inside a draw and a stale offset is a panic there.
func (a *App) chooserWindow() (int, int) {
	total := len(a.detailsEdit.options)
	visible := min(total, a.chooserVisibleRows())
	first := min(max(0, a.detailsEdit.offset), max(0, total-visible))
	return first, first + visible
}

// fieldChooserLines is the open chooser as page rows, and where the highlight
// landed. Framed like a comment card, hanging off the value column.
func (a *App) fieldChooserLines(column int) ([]string, int) {
	if a.detailsEdit.open == "" {
		return nil, -1
	}
	// Pulled back off a pane narrower than the metadata gutter. The value column
	// is a column of the untruncated row, and past the drawn line nothing shows.
	column = max(0, min(column, a.detailsFittedWidth-commentCardMinWidth))
	width := max(0, a.detailsFittedWidth-column)
	indent := strings.Repeat(" ", column)
	if width < commentCardMinWidth {
		// Too narrow to frame, the way a comment drops its border rather than
		// spending four cells of a pane this size on one.
		rows, highlight := a.chooserRows(width)
		for i, row := range rows {
			rows[i] = indent + a.litIf(row, i == highlight, width)
		}
		return rows, highlight
	}
	// Measured before the cursor line goes on, which pads a row out to the
	// full width and would make every list as wide as the pane.
	inner := width - commentCardChrome
	rows, highlight := a.chooserRows(inner)
	widest := 0
	for _, row := range rows {
		widest = max(widest, tview.TaggedStringWidth(row))
	}
	inner = min(inner, widest)
	width = inner + commentCardChrome
	// The accent, not the plain border: an open chooser holds the keyboard, the
	// same thing a lit comment card and every modal panel say with it.
	border := a.themeTags.BorderFocus
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, indent+cardEdge("╭", "╮", width, border))
	for i, row := range rows {
		lines = append(lines, indent+cardRow(a.litIf(row, i == highlight, inner), inner, border))
	}
	if highlight >= 0 {
		highlight++
	}
	return append(lines, indent+cardEdge("╰", "╯", width, border)), highlight
}

// litIf paints a row as the current one, padded so the cursor line runs the
// width of the list rather than the width of the word.
func (a *App) litIf(row string, lit bool, inner int) string {
	if !lit {
		return row
	}
	pad := max(0, inner-tview.TaggedStringWidth(row))
	return a.themeTags.Selection + row + strings.Repeat(" ", pad) + "[-:-:-]"
}

// chooserRows is the option text and which row is lit, unpainted and unpadded.
// A chooser with nothing to show yet says so on the one row it has.
func (a *App) chooserRows(inner int) ([]string, int) {
	fit := func(text string) string { return truncateTagged(text, max(1, inner)) }
	if a.detailsEdit.loading {
		return []string{fit(a.themeTags.SecondaryText + "Loading options[-]")}, -1
	}
	if len(a.detailsEdit.options) == 0 {
		return []string{fit(a.themeTags.SecondaryText + "No options[-]")}, -1
	}
	first, last := a.chooserWindow()
	rows := make([]string, 0, last-first+1)
	highlight := -1
	for i := first; i < last; i++ {
		label := a.detailsEdit.options[i].Label
		if i == a.detailsEdit.choice {
			// Left bare: the cursor line carries the color for this one.
			highlight = len(rows)
		} else {
			label = a.themeTags.Foreground + label + "[-]"
		}
		rows = append(rows, fit(label))
	}
	// What is below, not what is hidden: the row sits at the foot of the list,
	// so counting the ones scrolled off the top would point the wrong way.
	if rest := len(a.detailsEdit.options) - last; rest > 0 {
		rows = append(rows, fit(fmt.Sprintf("%s… +%d more[-]", a.themeTags.SecondaryText, rest)))
	}
	return rows, highlight
}

// handleChooserKey answers for the whole app while options are on the page. No
// command shortcut: an overlay over a chooser is two lists on one keyboard.
func (a *App) handleChooserKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		return event
	case tcell.KeyEscape:
		a.closeFieldChooser()
	case tcell.KeyEnter:
		a.commitFieldChooser()
	case tcell.KeyDown:
		a.moveChooserChoice(1)
	case tcell.KeyUp:
		a.moveChooserChoice(-1)
	case tcell.KeyRune:
		switch event.Rune() {
		case 'j':
			a.moveChooserChoice(1)
		case 'k':
			a.moveChooserChoice(-1)
		}
	}
	return nil
}
