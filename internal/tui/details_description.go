package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func (a *App) openDescriptionBox(description string) {
	if a.detailsDescArea == nil {
		return
	}
	a.detailsEdit.editing = issueFieldDescription
	a.detailsEdit.err = ""
	a.detailsEdit.gen = a.editGeneration.Add(1)
	a.detailsDescArea.SetText(description, false)
	a.detailsDescArea.SetOffset(0, 0)
	a.detailsFocus = detailsFocusDescription
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollEditorIntoView()
	a.updateStatusBar()
}

func (a *App) closeDescriptionBox() {
	if a.detailsEdit.editing != issueFieldDescription {
		return
	}
	a.detailsEdit.editing = ""
	a.detailsEdit.issue = linearapi.Issue{}
	a.detailsDescArea.SetText("", false)
	a.detailsFocus = detailsFocusCards
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

func (a *App) closeOpenEditor() {
	if a.detailsEdit.editing == issueFieldDescription {
		a.closeDescriptionBox()
		return
	}
	a.closeFieldEditor()
}

func (a *App) claimDescriptionFocus() {
	if a.focusedPane == FocusPalette || a.activeModal() != nil ||
		a.detailsEdit.editing != issueFieldDescription {
		return
	}
	a.detailsFocus = detailsFocusDescription
	a.applyPaneBorders()
	a.updateStatusBar()
}

// Sent untrimmed: leading whitespace is an indented code block to Linear.
func (a *App) commitDescription() {
	if a.detailsEdit.editing != issueFieldDescription || a.detailsDescArea == nil {
		return
	}
	body := a.detailsDescArea.GetText()
	if body == a.detailsEdit.issue.Description {
		a.closeDescriptionBox()
		return
	}
	issue := a.editTargetIssue()
	gen := a.detailsEdit.gen
	if _, out := a.savingDescriptions[gen]; out {
		a.flashStatus("Already saving this description")
		return
	}
	if a.savingDescriptions == nil {
		a.savingDescriptions = make(map[uint64]struct{})
	}
	a.savingDescriptions[gen] = struct{}{}
	a.saveIssueFieldWithResult(issueFieldDescriptionSave(issue, body), func(err error) {
		delete(a.savingDescriptions, gen)
		if err == nil && a.detailsEdit.editing == issueFieldDescription && a.detailsEdit.gen == gen {
			a.closeDescriptionBox()
		}
	})
}

func (a *App) refitDescriptionBox() {
	if a.detailsEdit.editing != issueFieldDescription || a.detailsPage == nil {
		return
	}
	for _, slot := range a.detailsPage.slots {
		if slot.primitive != a.detailsDescArea {
			continue
		}
		if writingBoxRows(a.detailsDescArea, slot.width) != slot.height {
			a.renderDetailsPage()
			a.scrollEditorIntoView()
		}
		return
	}
}

func descriptionBoxRect(width int) (column, inner int) {
	column = min(detailsCursorGutter, max(0, width-fieldEditorMinWidth))
	return column, max(0, width-column)
}

func (a *App) descriptionRail() string {
	return a.themeTags.Accent + detailsWriteMarker + "[-]"
}

func (a *App) descriptionLabelRow() string {
	label := a.themeTags.SecondaryText + "Description:[-]"
	row := detailsRow{text: label, field: issueFieldDescription, label: "Description"}
	return truncateTagged(a.fieldCursorMarker(row)+label, a.detailsFittedWidth)
}

func (a *App) editIssueDescription() {
	a.enterDetailsEdit()
	if !a.detailsEdit.on || a.fieldSpanIndex(issueFieldDescription) < 0 {
		return
	}
	a.detailsEdit.cursor = issueFieldDescription
	a.openFieldEditor()
}

func (a *App) handleDescriptionKey(event *tcell.EventKey) *tcell.EventKey {
	a.scrollEditorIntoView()
	switch event.Key() {
	case tcell.KeyCtrlC:
		if text, _, _ := a.detailsDescArea.GetSelection(); text != "" {
			a.copyText(text)
		}
		return nil
	case tcell.KeyEscape:
		a.closeDescriptionBox()
		return nil
	case tcell.KeyCtrlS:
		a.commitDescription()
		return nil
	case tcell.KeyEnter:
		if event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			a.commitDescription()
			return nil
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		return nil
	}
	return event
}
