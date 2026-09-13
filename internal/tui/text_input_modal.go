package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TextInputModal struct {
	app      *App
	fm       *FormModal
	input    *tview.InputField
	onSubmit func(string)
}

func NewTextInputModal(app *App) *TextInputModal {
	tm := &TextInputModal{app: app}
	tm.fm = NewFormModal(app, "Input")
	tm.input = tm.fm.AddInput("Value", "")
	tm.fm.SetOnCancel(tm.Hide)
	tm.fm.SetHint("⏎ save · Esc cancel")
	return tm
}

func (tm *TextInputModal) Show(title, label, initial string, onSubmit func(string)) {
	tm.ShowWithContext(title, label, initial, "", onSubmit)
}

func (tm *TextInputModal) ShowWithContext(title, label, initial, contextLine string, onSubmit func(string)) {
	tm.onSubmit = onSubmit
	tm.fm.SetTitle(title)
	tm.fm.SetRowLabel(0, label)
	tm.fm.SetContext(contextLine)
	tm.input.SetText(initial)
	tm.fm.Show("text_input")
}

func (tm *TextInputModal) Hide() {
	tm.fm.Hide("text_input")
}

func (tm *TextInputModal) Focus() { tm.fm.Focus() }

func (tm *TextInputModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModNone {
		value := strings.TrimSpace(tm.input.GetText())
		tm.Hide()
		if tm.onSubmit != nil {
			tm.onSubmit(value)
		}
		return nil
	}
	return tm.fm.HandleKey(event)
}

func (tm *TextInputModal) GetModal() *tview.Flex {
	return tm.fm.Root()
}
