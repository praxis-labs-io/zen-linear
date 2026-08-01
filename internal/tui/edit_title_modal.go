package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditTitleModal manages the edit title form overlay.
type EditTitleModal struct {
	app        *App
	fm         *FormModal
	titleField *tview.InputField
	issueID    string
	onUpdate   func(issueID, title string)
}

// NewEditTitleModal creates a new edit title modal.
func NewEditTitleModal(app *App) *EditTitleModal {
	etm := &EditTitleModal{app: app}
	etm.fm = NewFormModal(app, "Edit Title")
	etm.titleField = etm.fm.AddInput("Title", "")
	submit := func() {
		title := etm.titleField.GetText()
		etm.Hide()
		if etm.onUpdate != nil && title != "" && etm.issueID != "" {
			etm.onUpdate(etm.issueID, title)
		}
	}
	etm.fm.AddButtons(
		FormButton{Label: "Update", OnPress: submit},
		FormButton{Label: "Cancel", OnPress: etm.Hide},
	)
	etm.fm.SetOnSubmit(submit)
	etm.fm.SetOnCancel(etm.Hide)
	etm.fm.SetHint("Esc cancel · ⏎ next · ⌃⏎ submit")
	return etm
}

// Show displays the edit title modal. contextLine names the issue above the
// field.
func (etm *EditTitleModal) Show(issueID, currentTitle, contextLine string, onUpdate func(issueID, title string)) {
	etm.issueID = issueID
	etm.onUpdate = onUpdate
	etm.fm.SetContext(contextLine)
	etm.titleField.SetText(currentTitle)
	etm.fm.Show("edit_title")
}

// Hide hides the edit title modal.
func (etm *EditTitleModal) Hide() {
	etm.fm.Hide("edit_title")
}

// HandleKey handles keyboard input for the edit title modal.
func (etm *EditTitleModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return etm.fm.HandleKey(event)
}

// GetModal returns the modal flex for adding to pages.
func (etm *EditTitleModal) GetModal() *tview.Flex {
	return etm.fm.Root()
}
