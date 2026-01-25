package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditTitleModal manages the edit title form overlay.
type EditTitleModal struct {
	app        *App
	modal      *tview.Flex
	form       *tview.Form
	titleField *tview.InputField
	issueID    string
	onUpdate   func(issueID, title string)
}

// NewEditTitleModal creates a new edit title modal.
func NewEditTitleModal(app *App) *EditTitleModal {
	etm := &EditTitleModal{
		app: app,
	}

	// Create form
	etm.form = tview.NewForm()
	etm.form.SetBackgroundColor(app.theme.HeaderBg)
	etm.form.SetFieldBackgroundColor(app.theme.InputBg)
	etm.form.SetFieldTextColor(app.theme.Foreground)
	etm.form.SetButtonBackgroundColor(app.theme.Accent)
	etm.form.SetButtonTextColor(app.theme.SelectionText)
	etm.form.SetLabelColor(app.theme.Foreground)
	etm.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			etm.Hide()
			return nil
		}
		return event
	})

	// Add title field
	etm.titleField = tview.NewInputField()
	etm.titleField.SetLabel("Title: ")
	etm.titleField.SetFieldWidth(40)
	etm.titleField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			etm.Hide()
			return nil
		}
		return event
	})
	etm.form.AddFormItem(etm.titleField)

	// Add buttons
	etm.form.AddButton("Update", func() {
		title := etm.titleField.GetText()
		etm.Hide()
		if etm.onUpdate != nil && title != "" && etm.issueID != "" {
			etm.onUpdate(etm.issueID, title)
		}
	})
	etm.form.AddButton("Cancel", func() {
		etm.Hide()
	})

	// Create title
	titleView := tview.NewTextView()
	titleView.SetText("Edit Issue Title")
	titleView.SetTextColor(app.theme.Accent)
	titleView.SetBackgroundColor(app.theme.HeaderBg)

	// Build modal content
	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(titleView, 1, 0, false).
		AddItem(etm.form, 0, 1, true)
	modalContent.Box = tview.NewBox().SetBackgroundColor(app.theme.HeaderBg)
	modalContent.SetBackgroundColor(app.theme.HeaderBg).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
		SetTitle(" Edit Title ").
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	modalContent.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	// Center the modal on screen
	etm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, 8, 0, true).
			AddItem(nil, 0, 1, false), 60, 0, true).
		AddItem(nil, 0, 1, false)
	etm.modal.SetBackgroundColor(app.theme.Background)

	return etm
}

// Show displays the edit title modal.
func (etm *EditTitleModal) Show(issueID, currentTitle string, onUpdate func(issueID, title string)) {
	etm.issueID = issueID
	etm.onUpdate = onUpdate

	// Set current title in field
	etm.titleField.SetText(currentTitle)

	etm.app.pages.AddPage("edit_title", etm.modal, true, true)
	etm.app.pages.SendToFront("edit_title")
	etm.app.app.SetFocus(etm.form)
}

// Hide hides the edit title modal.
func (etm *EditTitleModal) Hide() {
	etm.app.pages.RemovePage("edit_title")
	etm.app.updateFocus()
}

// HandleKey handles keyboard input for the edit title modal.
func (etm *EditTitleModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		etm.Hide()
		return nil
	}
	return event
}

// GetModal returns the modal flex for adding to pages.
func (etm *EditTitleModal) GetModal() *tview.Flex {
	return etm.modal
}
