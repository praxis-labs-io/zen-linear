package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// openDescriptionBox turns the rendered body into a box holding its markdown,
// unframed. Called by openFieldEditor, which owns the guards the two share.
func (a *App) openDescriptionBox(description string) {
	if a.detailsDescArea == nil {
		return
	}
	a.detailsEdit.editing = issueFieldDescription
	a.detailsEdit.err = ""
	// Caret at the head, unlike every other box here: a description is read
	// before it is rewritten, and a long one opened on its tail is not.
	a.detailsDescArea.SetText(description, false)
	a.detailsDescArea.SetOffset(0, 0)
	a.detailsFocus = detailsFocusDescription
	// Rendered before the focus moves: the slot has to be on the page for the
	// keyboard to land in it.
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollEditorIntoView()
	a.updateStatusBar()
}

// closeDescriptionBox puts the rendered body back and leaves edit mode on. The
// words go with it: nothing holds a rewrite, the way nothing holds a comment's.
func (a *App) closeDescriptionBox() {
	if a.detailsEdit.editing != issueFieldDescription {
		return
	}
	a.detailsEdit.editing = ""
	a.detailsEdit.issue = linearapi.Issue{}
	a.detailsDescArea.SetText("", false)
	a.detailsFocus = detailsFocusCards
	// Rendered before the focus moves off, or the caret sits on a widget whose
	// rect this render is about to zero.
	a.renderDetailsPage()
	a.updateFocus()
	a.scrollFieldIntoView()
	a.updateStatusBar()
}

// closeOpenEditor shuts whichever box is open, and does nothing when none is.
func (a *App) closeOpenEditor() {
	if a.detailsEdit.editing == issueFieldDescription {
		a.closeDescriptionBox()
		return
	}
	a.closeFieldEditor()
}

// claimDescriptionFocus records the keyboard landing in the box by mouse, and
// moves none itself. Not enterDetailsFocus, whose guard would drop the mode.
func (a *App) claimDescriptionFocus() {
	if a.focusedPane == FocusPalette || a.activeModal() != nil ||
		a.detailsEdit.editing != issueFieldDescription {
		return
	}
	a.detailsFocus = detailsFocusDescription
	a.applyPaneBorders()
	a.updateStatusBar()
}

// commitDescription sends what was typed. The box stays open until Linear
// answers, since a rewrite has nowhere else to be held.
func (a *App) commitDescription() {
	if a.detailsEdit.editing != issueFieldDescription || a.detailsDescArea == nil {
		return
	}
	// Sent as written: leading whitespace is an indented code block to Linear,
	// so trimming would rewrite a description the user only looked at.
	body := a.detailsDescArea.GetText()
	// Measured against the issue the box opened on, so sending it unchanged is
	// a close rather than a write that undoes what a refresh brought in.
	if body == a.detailsEdit.issue.Description {
		a.closeDescriptionBox()
		return
	}
	issue := a.editTargetIssue()
	if issue.ID != "" && issue.ID == a.savingDescription {
		a.flashStatus("Already saving this description")
		return
	}
	a.savingDescription = issue.ID
	a.saveIssueFieldWithResult(issueFieldDescriptionSave(issue, body), func(err error) {
		a.savingDescription = ""
		// The box may be about another issue by now, and closing it then would
		// throw away words written against that one.
		if err == nil && a.detailsEdit.editing == issueFieldDescription &&
			a.detailsEdit.issue.ID == issue.ID {
			a.closeDescriptionBox()
		}
	})
}

// refitDescriptionBox re-renders when what has been typed no longer fits the
// rows the box was drawn with, which is how it grows with its own text.
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
			// The page scrolled to the box before the key that grew it landed,
			// so without this the row just gained sits below the fold.
			a.scrollEditorIntoView()
		}
		return
	}
}

// descriptionRail is the write marker on one row of the open box, so the bar
// runs the height of what is being written rather than heading it alone.
func (a *App) descriptionRail() string {
	return a.themeTags.Accent + detailsWriteMarker + "[-]"
}

// descriptionLabelRow heads the description, carrying the field cursor. Emitted
// here, since renderDetailsBody is rebuilt on a width change and not a move.
func (a *App) descriptionLabelRow() string {
	label := a.themeTags.SecondaryText + "Description:[-]"
	row := detailsRow{text: label, field: issueFieldDescription, label: "Description"}
	return truncateTagged(a.fieldCursorMarker(row)+label, a.detailsFittedWidth)
}

// editIssueDescription is the command's route into the box: edit mode on, the
// cursor on the description, the box open.
func (a *App) editIssueDescription() {
	a.enterDetailsEdit()
	if !a.detailsEdit.on || a.fieldSpanIndex(issueFieldDescription) < 0 {
		return
	}
	a.detailsEdit.cursor = issueFieldDescription
	a.openFieldEditor()
}

// handleDescriptionKey answers for the whole app while the box is on the page,
// default-allow: it is prose, so q is a letter and Enter a newline.
func (a *App) handleDescriptionKey(event *tcell.EventKey) *tcell.EventKey {
	// The wheel goes straight through the pane, and a box scrolled off the top
	// is not drawn while it still takes every key. The compose path does this.
	a.scrollEditorIntoView()
	switch event.Key() {
	case tcell.KeyCtrlC:
		// Copy, never quit: quitting here costs the rewrite. Swallowed either
		// way, since tview stops the app on one handed back.
		if text, _, _ := a.detailsDescArea.GetSelection(); text != "" {
			a.copyText(text)
		}
		return nil
	case tcell.KeyEscape:
		a.closeDescriptionBox()
		return nil
	case tcell.KeyEnter:
		// Enter is a newline in prose, which is the only thing it can be, so
		// the chord is the way to send without leaving the words.
		if event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			a.commitDescription()
			return nil
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		// Swallowed: the box has no second control, and tview would walk the
		// keyboard to whatever primitive comes next.
		return nil
	}
	return event
}
