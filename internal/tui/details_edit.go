package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// detailsCursorGutter is the two cells edit mode reserves at the head of every
// header row for the cursor's marker.
const detailsCursorGutter = 2

// detailsEditState is the pane's edit mode: whether it is on, and the field the
// cursor points at.
type detailsEditState struct {
	on bool
	// An id, never a row index: the header is rebuilt under the cursor by every
	// background refresh.
	cursor issueField
}

// enterDetailsEdit puts the pane in edit mode with the cursor on the first
// field, revealing and focusing the pane the way openComposeBox does.
func (a *App) enterDetailsEdit() {
	issue := a.GetSelectedIssue()
	if a.detailsPage == nil || issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	if a.detailsEdit.on {
		// Already in it, and re-entering would throw the cursor back to the
		// first field.
		return
	}
	// The selection moves at once but the render rides the detail debounce, so
	// inside that window the page is still the issue before this one.
	if a.detailsIssueID != issue.ID {
		a.updateDetailsView()
	}
	cursor := a.firstEditableField()
	if cursor == "" {
		a.flashStatus("Nothing to edit on this issue")
		return
	}
	a.detailsHidden = false
	a.focusedPane = FocusDetails
	// The cards let go of the keyboard: two rings on one page would both answer
	// j and k.
	a.commentsFocus, a.focusedCommentID = commentsFocusCards, ""
	a.detailsEdit = detailsEditState{on: true, cursor: cursor}
	a.rebuildContentLayout()
	a.updateFocus()
	// Never leave the mode on over a pane that is not showing it, the way the
	// compose box backs out of a layout that did not put it on screen.
	if !a.detailsHaveFocus() {
		a.detailsEdit = detailsEditState{}
		a.updateFocus()
		return
	}
	// The marker is in the page text, so the mode shows nothing until the page
	// is written again.
	a.renderDetailsPage()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

// leaveDetailsEdit takes the pane out of edit mode. It moves no focus, so a
// focus callback can reach it.
func (a *App) leaveDetailsEdit() {
	if !a.detailsEdit.on {
		return
	}
	a.detailsEdit = detailsEditState{}
	a.renderDetailsPage()
	a.updateStatusBar()
}

// firstEditableField is the field the cursor starts on, empty when the page
// draws none.
func (a *App) firstEditableField() issueField {
	if len(a.detailsFieldSpans) == 0 {
		return ""
	}
	return a.detailsFieldSpans[0].field
}

// fieldSpanIndex finds a field on the page just rendered, -1 when it is not on
// it.
func (a *App) fieldSpanIndex(field issueField) int {
	if field == "" {
		return -1
	}
	for i, span := range a.detailsFieldSpans {
		if span.field == field {
			return i
		}
	}
	return -1
}

// stepFieldCursor moves the cursor one field down (+1) or up (-1). Off either
// end it stays where it is, the way the comment ring stops.
func (a *App) stepFieldCursor(step int) {
	if !a.detailsEdit.on || len(a.detailsFieldSpans) == 0 {
		return
	}
	next := 0
	if index := a.fieldSpanIndex(a.detailsEdit.cursor); index >= 0 {
		next = index + step
		if next < 0 || next >= len(a.detailsFieldSpans) {
			return
		}
	} else if step < 0 {
		// The cursor named a field this page does not draw, so it lands on the
		// end it was stepping toward rather than refusing the key.
		next = len(a.detailsFieldSpans) - 1
	}
	a.detailsEdit.cursor = a.detailsFieldSpans[next].field
	// Re-rendered rather than repainted, and the same call rebuilds the spans
	// the scroll below reads.
	a.renderDetailsPage()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

// resolveFieldCursor puts the cursor back on the page a rebuild just drew,
// looked up by id. A page with nothing to edit drops the mode.
func (a *App) resolveFieldCursor() {
	if !a.detailsEdit.on || a.fieldSpanIndex(a.detailsEdit.cursor) >= 0 {
		return
	}
	a.detailsEdit.cursor = a.firstEditableField()
	if a.detailsEdit.cursor == "" {
		a.detailsEdit = detailsEditState{}
	}
	// The page was written with a cursor it does not carry, so no row holds the
	// marker until it is written again.
	a.renderDetailsPage()
}

// scrollFieldIntoView brings the cursor's row onto the page, and does nothing
// when it is already there.
func (a *App) scrollFieldIntoView() {
	if index := a.fieldSpanIndex(a.detailsEdit.cursor); index >= 0 {
		row := a.detailsFieldSpans[index].row
		a.scrollRowsIntoView(row, row)
	}
}

// fieldCursorMarker heads one header row in edit mode: the cursor on its own
// field, the gutter it reserves elsewhere. A blank row takes neither.
func (a *App) fieldCursorMarker(row detailsRow) string {
	if !a.detailsEdit.on || row.text == "" {
		return ""
	}
	if row.field != "" && row.field == a.detailsEdit.cursor {
		return a.themeTags.Accent + "❯[-] "
	}
	return strings.Repeat(" ", detailsCursorGutter)
}

// handleDetailsEditKey answers for the whole app while edit mode is on. It is
// default-deny, so q cannot quit and a pane number cannot leave.
func (a *App) handleDetailsEditKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		// The one key the mode does not own. Handed back, tview stops the app
		// on it itself.
		return event
	case tcell.KeyEscape:
		a.leaveDetailsEdit()
	case tcell.KeyDown:
		a.stepFieldCursor(1)
	case tcell.KeyUp:
		a.stepFieldCursor(-1)
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'j':
			a.stepFieldCursor(1)
		case 'k':
			a.stepFieldCursor(-1)
		default:
			// The pickers keep their shortcuts. A modal takes the keys ahead of
			// this handler, so the mode waits under one rather than ending.
			a.runCommandShortcut(r)
		}
	}
	return nil
}
