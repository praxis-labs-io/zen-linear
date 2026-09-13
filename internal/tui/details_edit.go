package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

const detailsCursorGutter = 2

const detailsWriteMarker = "▌"

type detailsEditState struct {
	on      bool
	cursor  issueField
	open    issueField
	issue   linearapi.Issue
	options []PickerItem
	choice  int
	offset  int
	loading bool
	picked  map[string]bool
	editing issueField
	err     string
	gen     uint64
}

func (a *App) enterDetailsEdit() {
	issue := a.GetSelectedIssue()
	if a.detailsPage == nil || issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	if a.detailsEdit.on {
		return
	}
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
	a.detailsFocus, a.focusedCommentID = detailsFocusCards, ""
	a.detailsEdit = detailsEditState{on: true, cursor: cursor}
	a.rebuildContentLayout()
	a.updateFocus()
	if !a.detailsHaveFocus() {
		a.detailsEdit = detailsEditState{}
		a.updateFocus()
		return
	}
	a.renderDetailsPage()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

func (a *App) leaveDetailsEdit() {
	if !a.detailsEdit.on {
		return
	}
	a.releaseFieldEditor()
	a.detailsEdit = detailsEditState{}
	a.renderDetailsPage()
	a.updateStatusBar()
}

func (a *App) firstEditableField() issueField {
	if len(a.detailsFieldSpans) == 0 {
		return ""
	}
	return a.detailsFieldSpans[0].field
}

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
		next = len(a.detailsFieldSpans) - 1
	}
	a.detailsEdit.cursor = a.detailsFieldSpans[next].field
	a.renderDetailsPage()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

func (a *App) resolveFieldCursor() {
	if a.detailsEdit.on && a.detailsEdit.open != "" && a.fieldSpanIndex(a.detailsEdit.open) < 0 {
		a.closeFieldChooser()
	}
	if a.detailsEdit.on && a.detailsEdit.editing != "" && a.fieldSpanIndex(a.detailsEdit.editing) < 0 {
		a.closeOpenEditor()
	}
	if !a.detailsEdit.on || a.fieldSpanIndex(a.detailsEdit.cursor) >= 0 {
		return
	}
	a.detailsEdit.cursor = a.firstEditableField()
	if a.detailsEdit.cursor == "" {
		a.detailsEdit = detailsEditState{}
	}
	a.renderDetailsPage()
}

func (a *App) scrollFieldIntoView() {
	if index := a.fieldSpanIndex(a.detailsEdit.cursor); index >= 0 {
		row := a.detailsFieldSpans[index].row
		a.scrollRowsIntoView(row, row)
	}
}

func (a *App) fieldCursorMarker(row detailsRow) string {
	if !a.detailsEdit.on || row.text == "" {
		return ""
	}
	if row.field != "" && row.field == a.detailsEdit.cursor {
		tag, glyph := a.themeTags.Accent, "❯"
		switch {
		case a.detailsEdit.open != "":
			tag = a.themeTags.SecondaryText
		case a.detailsEdit.editing != "":
			glyph = detailsWriteMarker
		}
		return tag + glyph + "[-] "
	}
	return strings.Repeat(" ", detailsCursorGutter)
}

func (a *App) handleDetailsEditKey(event *tcell.EventKey) *tcell.EventKey {
	if a.detailsEdit.editing == issueFieldDescription {
		return a.handleDescriptionKey(event)
	}
	if a.detailsEdit.editing != "" {
		return a.handleFieldEditorKey(event)
	}
	if a.detailsEdit.open != "" {
		return a.handleChooserKey(event)
	}
	switch event.Key() {
	case tcell.KeyEnter:
		if fieldHasEditor(a.detailsEdit.cursor) {
			a.openFieldEditor()
		} else {
			a.openFieldChooser()
		}
	case tcell.KeyCtrlC:
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
			a.runCommandShortcut(r)
		}
	}
	return nil
}
