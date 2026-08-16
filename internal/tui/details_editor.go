package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// fieldHasEditor is the fields Enter opens a text box on, the ones typed rather
// than picked.
func fieldHasEditor(field issueField) bool {
	switch field {
	case issueFieldTitle, issueFieldDueDate, issueFieldEstimate:
		return true
	}
	return false
}

// openFieldEditor drops a box holding the cursor's field into the page under it.
func (a *App) openFieldEditor() {
	if !a.detailsEdit.on || a.detailsEdit.open != "" || a.detailsEdit.editing != "" {
		return
	}
	field := a.detailsEdit.cursor
	if !fieldHasEditor(field) || a.detailsFieldInput == nil {
		return
	}
	issue := a.GetSelectedIssue()
	// The page is what the box is about, and inside the detail debounce the
	// selection has already moved off it.
	if issue == nil || issue.ID != a.detailsIssueID {
		return
	}
	a.detailsEdit.editing = field
	a.detailsEdit.issue = *issue
	a.detailsEdit.err = ""
	a.detailsFieldInput.SetText(fieldEditorText(field, *issue))
	a.detailsFocus = detailsFocusField
	// Rendered before the focus moves: the slot has to be on the page for the
	// keyboard to land in it.
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollEditorIntoView()
	a.updateStatusBar()
}

// closeFieldEditor takes the box off the page and leaves edit mode on.
func (a *App) closeFieldEditor() {
	if a.detailsEdit.editing == "" {
		return
	}
	a.detailsEdit.editing = ""
	a.detailsEdit.issue = linearapi.Issue{}
	a.detailsEdit.err = ""
	a.detailsFieldInput.SetText("")
	a.detailsFocus = detailsFocusCards
	// Rendered before the focus moves off, or the caret sits on a widget whose
	// rect this render is about to zero.
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

// releaseFieldEditor takes the keyboard off a box that is going away, next
// event rather than now: a focus callback reaches here, and moving from one recurses.
func (a *App) releaseFieldEditor() {
	if a.detailsFocus != detailsFocusField {
		return
	}
	a.detailsFocus = detailsFocusCards
	a.layoutFocusStale = true
}

// claimFieldEditorFocus records the keyboard landing in the box by mouse, and
// moves none itself. Not enterDetailsFocus, whose guard would drop the mode.
func (a *App) claimFieldEditorFocus() {
	if a.focusedPane == FocusPalette || a.activeModal() != nil || a.detailsEdit.editing == "" {
		return
	}
	a.detailsFocus = detailsFocusField
	a.applyPaneBorders()
	a.updateStatusBar()
}

// commitFieldEditor saves what was typed and closes. A value Linear would
// refuse keeps the box, the text, and gains the reason under it.
func (a *App) commitFieldEditor() {
	field := a.detailsEdit.editing
	if field == "" {
		return
	}
	text := strings.TrimSpace(a.detailsFieldInput.GetText())
	// Measured against the issue the box opened on: Enter after typing nothing
	// is a close, not a write that undoes what a refresh brought in.
	if text == fieldEditorText(field, a.detailsEdit.issue) {
		a.closeFieldEditor()
		return
	}
	save, err := fieldEditorSave(field, a.editTargetIssue(), text)
	if err != nil {
		a.detailsEdit.err = err.Error()
		a.renderDetailsPage()
		a.scrollEditorIntoView()
		return
	}
	a.closeFieldEditor()
	a.saveIssueField(save)
}

// fieldEditorText is what the box opens holding, and what an unchanged commit
// is measured against. Empty for a field the issue does not carry.
func fieldEditorText(field issueField, issue linearapi.Issue) string {
	switch field {
	case issueFieldTitle:
		return strings.TrimSpace(issue.Title)
	case issueFieldDueDate:
		if issue.DueDate != nil {
			return strings.TrimSpace(*issue.DueDate)
		}
	case issueFieldEstimate:
		// estimateText, not formatEstimate, which prints "-" for nothing and
		// would then be typed back in as the value.
		return estimateText(issue.Estimate)
	}
	return ""
}

// fieldEditorSave builds one field's write from the text typed. Empty clears
// the two fields that have an empty state, checked before the parse.
func fieldEditorSave(field issueField, issue linearapi.Issue, text string) (issueFieldSave, error) {
	switch field {
	case issueFieldTitle:
		return issueFieldTitleSave(issue, text)
	case issueFieldDueDate:
		if text == "" {
			return issueFieldDueDateClear(issue), nil
		}
		return issueFieldDueDateSave(issue, text)
	case issueFieldEstimate:
		if text == "" {
			return issueFieldEstimateClear(issue), nil
		}
		return issueFieldEstimateSave(issue, text)
	}
	return issueFieldSave{}, fmt.Errorf("no editor for %s", issueFieldNames[field])
}

// editorSpan is where an open box landed: the field's own row, and the last row
// of the frame under it.
type editorSpan struct {
	start int
	end   int
}

// noEditorSpan is what a render with no box open records.
var noEditorSpan = editorSpan{start: -1, end: -1}

// scrollEditorIntoView brings the field's row and the box under it onto the
// page, so the label naming the field stays with the typing.
func (a *App) scrollEditorIntoView() {
	span := a.detailsEditorSpan
	if a.detailsEdit.editing == "" || span.start < 0 {
		return
	}
	a.scrollRowsIntoView(span.start, max(span.start, span.end))
}

// fieldEditorLines is the open box as page rows and the widget's place among
// them. Framed like a comment card, hanging off the value column.
func (a *App) fieldEditorLines(column int) ([]string, pageSlot) {
	// Pulled back off a pane narrower than the metadata gutter. The value column
	// is a column of the untruncated row, and past the drawn line nothing shows.
	column = max(0, min(column, a.detailsFittedWidth-commentCardMinWidth))
	width := max(0, a.detailsFittedWidth-column)
	indent := strings.Repeat(" ", column)
	border := a.themeTags.BorderFocus
	if width < commentCardMinWidth {
		// Too narrow to frame, the way a comment drops its border rather than
		// spending four cells of a pane this size on one.
		lines := []string{indent}
		if row, ok := a.fieldEditorError(width); ok {
			lines = append(lines, indent+row)
		}
		return lines, pageSlot{primitive: a.detailsFieldInput, row: 0, height: 1, column: column, width: width}
	}
	inner := width - commentCardChrome
	lines := []string{
		indent + cardEdge("╭", "╮", width, border),
		indent + cardRow("", inner, border),
	}
	if row, ok := a.fieldEditorError(inner); ok {
		lines = append(lines, indent+cardRow(row, inner, border))
	}
	lines = append(lines, indent+cardEdge("╰", "╯", width, border))

	// The chrome is a border cell and a pad cell, so the typing starts two in.
	const cardInset = commentCardChrome / 2
	return lines, pageSlot{
		primitive: a.detailsFieldInput,
		row:       1,
		height:    1,
		column:    column + cardInset,
		width:     max(inner, 0),
	}
}

// fieldEditorError is the refusal drawn under the box, and whether there is one.
func (a *App) fieldEditorError(width int) (string, bool) {
	if a.detailsEdit.err == "" {
		return "", false
	}
	return truncateTagged(a.themeTags.Error+tview.Escape(a.detailsEdit.err)+"[-]", max(1, width)), true
}

// handleFieldEditorKey answers for the whole app while a box is on the page,
// default-allow: the box is being typed in, so q is a letter.
func (a *App) handleFieldEditorKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		a.closeFieldEditor()
		return nil
	case tcell.KeyEnter:
		a.commitFieldEditor()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		// Swallowed: the box has no second control, and InputField's done func
		// would hand the keyboard to whatever tview walks to next.
		return nil
	}
	return event
}
