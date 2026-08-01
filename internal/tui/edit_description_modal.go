package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditDescriptionModal manages the edit description form overlay.
type EditDescriptionModal struct {
	app       *App
	fm        *FormModal
	bodyField *tview.TextArea
	issueID   string
	onUpdate  func(issueID, description string)
}

// NewEditDescriptionModal creates a new edit description modal.
func NewEditDescriptionModal(app *App) *EditDescriptionModal {
	edm := &EditDescriptionModal{app: app}
	edm.fm = NewFormModal(app, "Edit Description")
	edm.bodyField = edm.fm.AddTextArea("Description", "", 10)
	// An empty submission is allowed: it clears the description.
	submit := func() {
		description := edm.bodyField.GetText()
		edm.Hide()
		if edm.onUpdate != nil && edm.issueID != "" {
			edm.onUpdate(edm.issueID, description)
		}
	}
	edm.fm.AddButtons(
		FormButton{Label: "Update", OnPress: submit},
		FormButton{Label: "Cancel", OnPress: edm.Hide},
	)
	edm.fm.SetOnSubmit(submit)
	edm.fm.SetOnCancel(edm.Hide)
	edm.fm.SetHint("Esc cancel · ⌃⏎ submit")
	return edm
}

// Show displays the edit description modal prefilled with the current text.
// contextLine names the issue above the field.
func (edm *EditDescriptionModal) Show(issueID, currentDescription, contextLine string, onUpdate func(issueID, description string)) {
	edm.issueID = issueID
	edm.onUpdate = onUpdate
	edm.fm.SetContext(contextLine)
	edm.bodyField.SetText(currentDescription, true)
	edm.fm.Show("edit_description")
}

// Hide hides the edit description modal.
func (edm *EditDescriptionModal) Hide() {
	edm.fm.Hide("edit_description")
}

// HandleKey handles keyboard input for the edit description modal.
func (edm *EditDescriptionModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return edm.fm.HandleKey(event)
}

// GetModal returns the modal flex for adding to pages.
func (edm *EditDescriptionModal) GetModal() *tview.Flex {
	return edm.fm.Root()
}
