package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

const fieldEditorMinWidth = 8

func fieldHasEditor(field issueField) bool {
	switch field {
	case issueFieldTitle, issueFieldDueDate, issueFieldEstimate, issueFieldDescription:
		return true
	}
	return false
}

// showCaret goes through a goroutine because QueueUpdateDraw blocks when called on the event loop.
func (a *App) openFieldEditor() {
	if !a.detailsEdit.on || a.detailsEdit.open != "" || a.detailsEdit.editing != "" {
		return
	}
	field := a.detailsEdit.cursor
	if !fieldHasEditor(field) || a.detailsFieldInput == nil {
		return
	}
	issue := a.GetSelectedIssue()
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
	a.renderDetailsPage()
	a.updateFocus()
	go a.QueueUpdateDraw(a.showCaret)
	a.scrollEditorIntoView()
	a.updateStatusBar()
}

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

func (a *App) showCaret() {
	handler := a.detailsFieldInput.InputHandler()
	if handler == nil || a.detailsEdit.editing == "" {
		return
	}
	handler(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), func(tview.Primitive) {})
}

func (a *App) closeFieldEditor() {
	if a.detailsEdit.editing == "" {
		return
	}
	a.detailsEdit.editing = ""
	a.detailsEdit.issue = linearapi.Issue{}
	a.detailsEdit.err = ""
	a.detailsFieldInput.SetText("")
	a.detailsFocus = detailsFocusCards
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

func (a *App) releaseFieldEditor() {
	if a.detailsFocus != detailsFocusField && a.detailsFocus != detailsFocusDescription {
		return
	}
	a.detailsFocus = detailsFocusCards
	a.layoutFocusStale = true
}

func (a *App) claimFieldEditorFocus() {
	if a.focusedPane == FocusPalette || a.activeModal() != nil || a.detailsEdit.editing == "" {
		return
	}
	a.detailsFocus = detailsFocusField
	a.applyPaneBorders()
	a.updateStatusBar()
}

func (a *App) commitFieldEditor() {
	field := a.detailsEdit.editing
	if field == "" {
		return
	}
	text := strings.TrimSpace(a.detailsFieldInput.GetText())
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

func fieldEditorText(field issueField, issue linearapi.Issue) string {
	switch field {
	case issueFieldTitle:
		return strings.TrimSpace(issue.Title)
	case issueFieldDueDate:
		if issue.DueDate != nil {
			return strings.TrimSpace(*issue.DueDate)
		}
	case issueFieldEstimate:
		return estimateText(issue.Estimate)
	}
	return ""
}

func estimateText(estimate *float64) string {
	if estimate == nil {
		return ""
	}
	return strconv.FormatFloat(*estimate, 'f', -1, 64)
}

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

type editorSpan struct {
	start int
	end   int
}

var noEditorSpan = editorSpan{start: -1, end: -1}

func (a *App) scrollEditorIntoView() {
	span := a.detailsEditorSpan
	if a.detailsEdit.editing == "" || span.start < 0 {
		return
	}
	a.scrollRowsIntoView(span.start, max(span.start, span.end))
}

func (a *App) fieldEditorRect(column int) (int, int) {
	column = max(0, min(column, max(0, a.detailsFittedWidth-fieldEditorMinWidth)))
	return column, max(0, a.detailsFittedWidth-column)
}

func (a *App) fieldEditorStyle(field issueField) tcell.Style {
	style := tcell.StyleDefault.Foreground(a.theme.Foreground).Background(a.theme.Background)
	return style.Bold(field == issueFieldTitle)
}

func (a *App) fieldEditorError(column int) (string, bool) {
	if a.detailsEdit.err == "" {
		return "", false
	}
	text := a.themeTags.Error + tview.Escape(a.detailsEdit.err) + "[-]"
	return truncateTagged(strings.Repeat(" ", column)+text, a.detailsFittedWidth), true
}

func (a *App) handleFieldEditorKey(event *tcell.EventKey) *tcell.EventKey {
	a.scrollEditorIntoView()
	switch event.Key() {
	case tcell.KeyEscape:
		a.closeFieldEditor()
		return nil
	case tcell.KeyEnter:
		a.commitFieldEditor()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		return nil
	}
	return event
}
