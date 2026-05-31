package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TextInputModal manages a single-field modal for small command inputs.
type TextInputModal struct {
	app      *App
	modal    *tview.Flex
	input    *tview.InputField
	helpView *tview.TextView
	onSubmit func(string)
}

// NewTextInputModal creates a new text input modal.
func NewTextInputModal(app *App) *TextInputModal {
	tm := &TextInputModal{app: app}

	tm.input = tview.NewInputField().
		SetFieldBackgroundColor(app.theme.Background).
		SetFieldTextColor(app.theme.Foreground).
		SetLabelColor(app.theme.SecondaryText)
	tm.input.SetBackgroundColor(app.theme.HeaderBg)

	tm.helpView = tview.NewTextView()
	tm.helpView.SetText("Enter: save | Esc: cancel")
	tm.helpView.SetTextColor(app.theme.SecondaryText)
	tm.helpView.SetBackgroundColor(app.theme.HeaderBg)
	tm.helpView.SetTextAlign(tview.AlignCenter)

	content := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(tm.input, 1, 0, true).
		AddItem(tm.helpView, 1, 0, false)
	content.Box = tview.NewBox().SetBackgroundColor(app.theme.HeaderBg)
	content.SetBackgroundColor(app.theme.HeaderBg).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	content.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	tm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(content, 7, 0, true).
			AddItem(nil, 0, 1, false), 60, 0, true).
		AddItem(nil, 0, 1, false)
	tm.modal.SetBackgroundColor(app.theme.Background)

	return tm
}

// Show displays the text input modal.
func (tm *TextInputModal) Show(title, label, initial string, onSubmit func(string)) {
	tm.onSubmit = onSubmit
	tm.input.SetLabel(label)
	tm.input.SetText(initial)
	tm.input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			value := strings.TrimSpace(tm.input.GetText())
			tm.Hide()
			if tm.onSubmit != nil {
				tm.onSubmit(value)
			}
		case tcell.KeyEscape:
			tm.Hide()
		}
	})

	if content, ok := tm.modal.GetItem(1).(*tview.Flex); ok && content.GetItemCount() > 1 {
		if inner, ok := content.GetItem(1).(*tview.Flex); ok {
			inner.SetTitle(" " + title + " ")
		}
	}
	tm.app.pages.AddPage("text_input", tm.modal, true, true)
	tm.app.pages.SendToFront("text_input")
	tm.app.app.SetFocus(tm.input)
}

// Hide hides the text input modal.
func (tm *TextInputModal) Hide() {
	tm.app.pages.RemovePage("text_input")
	tm.app.updateFocus()
}

// HandleKey handles keyboard input for the text input modal.
func (tm *TextInputModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		tm.Hide()
		return nil
	}
	return event
}
