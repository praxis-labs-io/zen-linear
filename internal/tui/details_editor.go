package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// fieldEditorMinWidth is what a box is never drawn narrower than, however far
// into a small pane its value column falls.
const fieldEditorMinWidth = 8

// fieldHasEditor is the fields Enter opens a text box on, the ones typed rather
// than picked. The description opens a many-row one; the rest a single row.
func fieldHasEditor(field issueField) bool {
	switch field {
	case issueFieldTitle, issueFieldDueDate, issueFieldEstimate, issueFieldDescription:
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
	if field == issueFieldDescription {
		a.detailsEdit.issue = *issue
		a.openDescriptionBox(issue.Description)
		return
	}
	a.detailsEdit.editing = field
	a.detailsEdit.issue = *issue
	a.detailsEdit.err = ""
	a.detailsFieldInput.SetText(fieldEditorText(field, *issue))
	a.detailsFieldInput.SetFieldStyle(a.fieldEditorStyle(field))
	a.detailsFocus = detailsFocusField
	// Rendered before the focus moves: the slot has to be on the page for the
	// keyboard to land in it.
	a.renderDetailsPage()
	a.updateFocus()
	// Queued from off the loop: the scroll wants a width only the next draw
	// gives, and QueueUpdate blocks on a loop that is us. See CLAUDE.md.
	go a.QueueUpdateDraw(a.showCaret)
	a.scrollEditorIntoView()
	a.updateStatusBar()
}

// fieldEditorSlot is the box's place on the page just rendered.
func (a *App) fieldEditorSlot() (pageSlot, bool) {
	if a.detailsPage == nil {
		return pageSlot{}, false
	}
	for _, slot := range a.detailsPage.slots {
		if slot.primitive == a.detailsFieldInput {
			return slot, true
		}
	}
	return pageSlot{}, false
}

// showCaret scrolls the end of the value onto the row, where SetText leaves the
// caret sitting off the right of a long one. See CLAUDE.md for why it is a key.
func (a *App) showCaret() {
	handler := a.detailsFieldInput.InputHandler()
	if handler == nil || a.detailsEdit.editing == "" {
		return
	}
	handler(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), func(tview.Primitive) {})
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
	if a.detailsFocus != detailsFocusField && a.detailsFocus != detailsFocusDescription {
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

// estimateText is what the box opens holding. formatEstimate is the table's
// and prints "-" for nothing, which would then be typed back in as the value.
func estimateText(estimate *float64) string {
	if estimate == nil {
		return ""
	}
	return strconv.FormatFloat(*estimate, 'f', -1, 64)
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
// the box costs the page under it.
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

// fieldEditorRect is where the box draws on the field's own row: the value's
// column, and what is left of the measure.
func (a *App) fieldEditorRect(column int) (int, int) {
	// Pulled back off a pane narrower than the metadata gutter. The value column
	// is a column of the untruncated row, and past the drawn line nothing shows.
	column = max(0, min(column, max(0, a.detailsFittedWidth-fieldEditorMinWidth)))
	return column, max(0, a.detailsFittedWidth-column)
}

// fieldEditorStyle draws the value the way the row reads it, unfilled, so
// typing into an issue looks like the issue and not like a form.
func (a *App) fieldEditorStyle(field issueField) tcell.Style {
	style := tcell.StyleDefault.Foreground(a.theme.Foreground).Background(a.theme.Background)
	return style.Bold(field == issueFieldTitle)
}

// fieldEditorError is the refusal drawn under the field, and whether there is
// one. Indented to the box it belongs to.
func (a *App) fieldEditorError(column int) (string, bool) {
	if a.detailsEdit.err == "" {
		return "", false
	}
	text := a.themeTags.Error + tview.Escape(a.detailsEdit.err) + "[-]"
	return truncateTagged(strings.Repeat(" ", column)+text, a.detailsFittedWidth), true
}

// handleFieldEditorKey answers for the whole app while a box is on the page,
// default-allow: the box is being typed in, so q is a letter.
func (a *App) handleFieldEditorKey(event *tcell.EventKey) *tcell.EventKey {
	// The wheel goes straight through the pane, and a box scrolled off the top
	// is not drawn while it still takes every key. The compose path does this.
	a.scrollEditorIntoView()
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
