package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

const (
	detailsChooserMaxRows = 10
	detailsChooserChrome  = 3
)

var chooserClearLabels = map[issueField]string{
	issueFieldAssignee:  "Unassigned",
	issueFieldProject:   "No project",
	issueFieldMilestone: "No milestone",
	issueFieldCycle:     "No cycle",
}

func fieldHasChooser(field issueField) bool {
	switch field {
	case issueFieldState, issueFieldAssignee, issueFieldPriority, issueFieldTeam,
		issueFieldProject, issueFieldMilestone, issueFieldCycle, issueFieldLabels:
		return true
	}
	return false
}

func (a *App) openFieldChooser() {
	if !a.detailsEdit.on || a.detailsEdit.open != "" || !fieldHasChooser(a.detailsEdit.cursor) {
		return
	}
	issue := a.GetSelectedIssue()
	if issue == nil || issue.ID != a.detailsIssueID {
		return
	}
	field := a.detailsEdit.cursor
	gen := a.editGeneration.Add(1)
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

// The generation lives on App so a mode exit cannot restart it at zero.
func (a *App) chooserIsCurrent(gen uint64, field issueField) bool {
	edit := a.detailsEdit
	return edit.on && edit.open == field && edit.gen == gen && edit.issue.ID == a.detailsIssueID
}

func (a *App) fillFieldChooser(gen uint64, field issueField, items []PickerItem) {
	if !a.chooserIsCurrent(gen, field) {
		return
	}
	current := currentFieldOptionID(field, a.detailsEdit.issue)
	if label, ok := chooserClearLabels[field]; ok && current != "" {
		items = append([]PickerItem{{Label: label}}, items...)
	}
	a.detailsEdit.loading = false
	a.detailsEdit.options = items
	if field == issueFieldLabels {
		a.detailsEdit.picked = make(map[string]bool, len(a.detailsEdit.issue.Labels))
		for _, label := range a.detailsEdit.issue.Labels {
			a.detailsEdit.picked[label.ID] = true
		}
	}
	a.detailsEdit.choice = chooserIndexOf(items, current)
	a.detailsEdit.offset = max(0, a.detailsEdit.choice-a.chooserVisibleRows()+1)
	a.renderDetailsPage()
	a.scrollChooserIntoView()
}

func (a *App) failFieldChooser(gen uint64, field issueField, err error) {
	if !a.chooserIsCurrent(gen, field) {
		return
	}
	a.closeFieldChooser()
	a.updateStatusBarWithError(err)
}

func (a *App) closeFieldChooser() {
	if a.detailsEdit.open == "" {
		return
	}
	a.detailsEdit.open = ""
	a.detailsEdit.issue = linearapi.Issue{}
	a.detailsEdit.options = nil
	a.detailsEdit.choice, a.detailsEdit.offset = 0, 0
	a.detailsEdit.loading = false
	a.detailsEdit.picked = nil
	a.renderDetailsPage()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

func (a *App) commitFieldChooser() {
	edit := a.detailsEdit
	if edit.open == "" || edit.loading || edit.choice < 0 || edit.choice >= len(edit.options) {
		return
	}
	picked := pickedIDs(edit.picked)
	issue := a.editTargetIssue()
	a.closeFieldChooser()
	if chooserUnchanged(edit, issue, picked) {
		return
	}
	if moved := chooserScopeMoved(edit.open, edit.issue, issue); moved != "" {
		a.flashError("The " + moved + " changed, pick again")
		return
	}
	if save, ok := chooserSave(edit, issue, picked); ok {
		a.saveIssueField(save)
	}
}

func pickedIDs(picked map[string]bool) []string {
	ids := make([]string, 0, len(picked))
	for id := range picked {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Rebased at commit: a label added while the chooser was open has no row, so the reader never decided to drop it.
func labelWrite(picked []string, options []PickerItem, issue linearapi.Issue) []string {
	offered := make(map[string]bool, len(options))
	for _, option := range options {
		offered[option.ID] = true
	}
	ids := make([]string, 0, len(picked))
	for _, id := range picked {
		if offered[id] {
			ids = append(ids, id)
		}
	}
	for _, label := range issue.Labels {
		if !offered[label.ID] {
			ids = append(ids, label.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func chooserUnchanged(edit detailsEditState, now linearapi.Issue, picked []string) bool {
	if edit.open == issueFieldLabels {
		return slices.Equal(picked, issueLabelIDs(edit.issue))
	}
	return edit.options[edit.choice].ID == currentFieldOptionID(edit.open, now)
}

func chooserScopeMoved(field issueField, opened, now linearapi.Issue) string {
	switch field {
	case issueFieldPriority, issueFieldTeam:
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

func (a *App) editTargetIssue() linearapi.Issue {
	opened := a.detailsEdit.issue
	a.issuesMu.RLock()
	selected := a.selectedIssue
	a.issuesMu.RUnlock()
	if selected != nil && selected.ID == opened.ID {
		return *selected
	}
	return opened
}

func chooserSave(edit detailsEditState, issue linearapi.Issue, picked []string) (issueFieldSave, bool) {
	item := edit.options[edit.choice]
	switch edit.open {
	case issueFieldLabels:
		return issueFieldLabelsSave(issue, labelWrite(picked, edit.options, issue)), true
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
	case issueFieldTeam:
		return issueFieldTeamSave(issue, item.ID, item.name()), true
	}
	return issueFieldSave{}, false
}

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
	case issueFieldTeam:
		return issue.TeamID
	}
	return ""
}

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

// Writes nothing: labelIds is one field, so a save per toggle races itself.
func (a *App) toggleChooserPick() {
	edit := &a.detailsEdit
	if edit.open != issueFieldLabels || edit.choice < 0 || edit.choice >= len(edit.options) {
		return
	}
	id := edit.options[edit.choice].ID
	if edit.picked[id] {
		delete(edit.picked, id)
	} else {
		edit.picked[id] = true
	}
	a.renderDetailsPage()
	a.scrollChooserIntoView()
}

type chooserSpan struct {
	lit int
	end int
}

var noChooserSpan = chooserSpan{lit: -1, end: -1}

func (a *App) scrollChooserIntoView() {
	span := a.detailsChooserSpan
	if a.detailsEdit.open == "" || span.lit < 0 {
		return
	}
	a.scrollRowsIntoView(span.lit, max(span.lit, span.end))
}

// Reads the height the last refit was handed, since the view's own rect is still the previous frame's during a draw.
func (a *App) chooserVisibleRows() int {
	rows := detailsChooserMaxRows
	if height := a.detailsFittedHeight; height > 0 {
		rows = min(rows, max(1, height-detailsChooserChrome))
	}
	return rows
}

// Clamped here because this runs inside a draw, where a stale offset panics.
func (a *App) chooserWindow() (int, int) {
	total := len(a.detailsEdit.options)
	visible := min(total, a.chooserVisibleRows())
	first := min(max(0, a.detailsEdit.offset), max(0, total-visible))
	return first, first + visible
}

func (a *App) fieldChooserLines(column int) ([]string, int) {
	if a.detailsEdit.open == "" {
		return nil, -1
	}
	column = max(0, min(column, a.detailsFittedWidth-commentCardMinWidth))
	width := max(0, a.detailsFittedWidth-column)
	indent := strings.Repeat(" ", column)
	if width < commentCardMinWidth {
		rows, highlight := a.chooserRows(width)
		for i, row := range rows {
			rows[i] = indent + a.litIf(row, i == highlight, width)
		}
		return rows, highlight
	}
	inner := width - commentCardChrome
	rows, highlight := a.chooserRows(inner)
	widest := 0
	for _, row := range rows {
		widest = max(widest, tview.TaggedStringWidth(row))
	}
	inner = min(inner, widest)
	width = inner + commentCardChrome
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

func (a *App) litIf(row string, lit bool, inner int) string {
	if !lit {
		return row
	}
	pad := max(0, inner-tview.TaggedStringWidth(row))
	return a.themeTags.Selection + row + strings.Repeat(" ", pad) + "[-:-:-]"
}

func (a *App) chooserRows(inner int) ([]string, int) {
	fit := func(text string) string { return truncateTagged(text, max(1, inner)) }
	if a.detailsEdit.loading {
		return []string{fit(a.themeTags.SecondaryText + "Loading options[-]")}, -1
	}
	if len(a.detailsEdit.options) == 0 {
		return []string{fit(a.themeTags.SecondaryText + "No options[-]")}, -1
	}
	first, last := a.chooserWindow()
	multi := a.detailsEdit.open == issueFieldLabels
	rows := make([]string, 0, last-first+1)
	highlight := -1
	for i := first; i < last; i++ {
		option := a.detailsEdit.options[i]
		label := option.Label
		if i == a.detailsEdit.choice {
			highlight = len(rows)
			if multi {
				label = multiSelectGlyph(a.detailsEdit.picked[option.ID]) + " " + label
			}
		} else {
			label = a.themeTags.Foreground + label + "[-]"
			if multi {
				label = a.multiSelectRow(label, a.detailsEdit.picked[option.ID])
			}
		}
		rows = append(rows, fit(label))
	}
	if rest := len(a.detailsEdit.options) - last; rest > 0 {
		rows = append(rows, fit(fmt.Sprintf("%s… +%d more[-]", a.themeTags.SecondaryText, rest)))
	}
	return rows, highlight
}

// Default-deny with no command shortcuts: an overlay over a chooser is two lists on one keyboard.
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
		case ' ':
			a.toggleChooserPick()
		case a.keysReferenceKey():
			a.ShowKeysModal()
		}
	}
	return nil
}
