package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfirmationModal manages a small confirm/cancel overlay for risky actions.
type ConfirmationModal struct {
	app       *App
	modal     *tview.Modal
	onConfirm func()
}

// NewConfirmationModal creates a confirmation modal.
func NewConfirmationModal(app *App) *ConfirmationModal {
	return &ConfirmationModal{app: app}
}

// Show displays the confirmation prompt.
func (cm *ConfirmationModal) Show(title, message, confirmLabel string, onConfirm func()) {
	cm.onConfirm = onConfirm
	cm.modal = tview.NewModal().
		SetText(message).
		AddButtons([]string{confirmLabel, "Cancel"}).
		SetDoneFunc(func(buttonIndex int, _ string) {
			confirmed := buttonIndex == 0
			cm.Hide()
			if confirmed && cm.onConfirm != nil {
				cm.onConfirm()
			}
		})
	cm.modal.SetBackgroundColor(cm.app.theme.HeaderBg)
	cm.modal.SetTextColor(cm.app.theme.Foreground)
	cm.modal.SetButtonBackgroundColor(cm.app.theme.Accent)
	cm.modal.SetButtonTextColor(cm.app.theme.SelectionText)
	cm.modal.SetBorder(true).
		SetBorderColor(cm.app.theme.Accent).
		SetTitle(" " + title + " ").
		SetTitleColor(cm.app.theme.Foreground)

	cm.app.pages.AddPage("confirmation", cm.modal, true, true)
	cm.app.pages.SendToFront("confirmation")
	cm.app.app.SetFocus(cm.modal)
}

// Hide closes the confirmation prompt.
func (cm *ConfirmationModal) Hide() {
	cm.app.pages.RemovePage("confirmation")
	cm.app.updateFocus()
}

// HandleKey handles confirmation-level shortcuts.
func (cm *ConfirmationModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		cm.Hide()
		return nil
	}
	return event
}
