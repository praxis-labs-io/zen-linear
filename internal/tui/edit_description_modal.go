package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditDescriptionModal manages the edit description form overlay.
type EditDescriptionModal struct {
	app       *App
	modal     *tview.Flex
	form      *tview.Form
	bodyField *tview.TextArea
	issueID   string
	onUpdate  func(issueID, description string)
}

// NewEditDescriptionModal creates a new edit description modal.
func NewEditDescriptionModal(app *App) *EditDescriptionModal {
	edm := &EditDescriptionModal{
		app: app,
	}

	// Create form
	edm.form = tview.NewForm()
	edm.form.SetBackgroundColor(app.theme.ModalBackground())
	edm.form.SetFieldBackgroundColor(app.theme.InputBg)
	edm.form.SetFieldTextColor(app.theme.Foreground)
	edm.form.SetButtonBackgroundColor(app.theme.Accent)
	edm.form.SetButtonTextColor(app.theme.SelectionText)
	edm.form.SetLabelColor(app.theme.Foreground)

	// Add description body field
	edm.form.AddTextArea("Description", "", 60, 10, 0, nil)
	if item := edm.form.GetFormItemByLabel("Description"); item != nil {
		if textArea, ok := item.(*tview.TextArea); ok {
			edm.bodyField = textArea
		}
	}

	// Add action buttons. An empty submission is allowed: it clears the
	// description.
	edm.form.AddButton("Update", func() {
		description := edm.bodyField.GetText()
		edm.Hide()
		if edm.onUpdate != nil && edm.issueID != "" {
			edm.onUpdate(edm.issueID, description)
		}
	})
	edm.form.AddButton("Cancel", func() {
		edm.Hide()
	})

	// Create header with instructions
	headerView := tview.NewTextView()
	headerView.SetText("Edit Issue Description")
	headerView.SetTextColor(app.theme.Accent)
	headerView.SetBackgroundColor(app.theme.ModalBackground())

	// Create help text
	helpView := tview.NewTextView()
	helpView.SetText("Esc: cancel • Ctrl+Enter / Cmd+Enter: submit")
	helpView.SetTextColor(app.theme.SecondaryText)
	helpView.SetBackgroundColor(app.theme.ModalBackground())
	helpView.SetTextAlign(tview.AlignCenter)

	// Build modal content
	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(headerView, 1, 0, false).
		AddItem(edm.form, 0, 1, true).
		AddItem(helpView, 1, 0, false)
	modalContent.Box = tview.NewBox().SetBackgroundColor(app.theme.ModalBackground())
	modalContent.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
		SetTitle(" Edit Description ").
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	modalContent.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	// Center the modal on screen
	edm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, 20, 0, true).
			AddItem(nil, 0, 1, false), 75, 0, true).
		AddItem(nil, 0, 1, false)
	edm.modal.SetBackgroundColor(app.theme.Background)

	return edm
}

// Show displays the edit description modal prefilled with the current text.
func (edm *EditDescriptionModal) Show(issueID, currentDescription string, onUpdate func(issueID, description string)) {
	edm.issueID = issueID
	edm.onUpdate = onUpdate

	edm.bodyField.SetText(currentDescription, true)
	// The form remembers its last focused item across shows; without this a
	// prior submit leaves focus on the Update button instead of the text.
	edm.form.SetFocus(0)

	edm.app.pages.AddPage("edit_description", edm.modal, true, true)
	edm.app.pages.SendToFront("edit_description")
	edm.app.app.SetFocus(edm.form)
}

// Hide hides the edit description modal.
func (edm *EditDescriptionModal) Hide() {
	edm.app.pages.RemovePage("edit_description")
	edm.app.updateFocus()
}

// HandleKey handles keyboard input for the edit description modal.
func (edm *EditDescriptionModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		edm.Hide()
		return nil
	case tcell.KeyEnter:
		// Ctrl+Enter or Cmd+Enter submits; plain Enter stays a newline.
		mod := event.Modifiers()
		if mod&tcell.ModCtrl != 0 || mod&tcell.ModMeta != 0 {
			description := edm.bodyField.GetText()
			edm.Hide()
			if edm.onUpdate != nil && edm.issueID != "" {
				edm.onUpdate(edm.issueID, description)
			}
			return nil
		}
	}
	return event
}

// GetModal returns the modal flex for adding to pages.
func (edm *EditDescriptionModal) GetModal() *tview.Flex {
	return edm.modal
}
