package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestFormModalShowResetsFocusToFirstField guards the whole bug class where
// reopening a modal leaves keyboard focus on whatever was focused last
// (tview forms remember their last-focused item across shows).
func TestFormModalShowResetsFocusToFirstField(t *testing.T) {
	app := newUXTestApp()
	fm := NewFormModal(app, "Test")
	first := fm.AddInput("Title", "")
	fm.AddTextArea("Body", "", 5)
	fm.AddButtons(FormButton{Label: "OK"}, FormButton{Label: "Cancel"})

	fm.Show("form_test")
	if app.app.GetFocus() != first {
		t.Fatal("first Show did not focus the first field")
	}
	// Simulate a prior session ending focused on a button, then reopen.
	app.app.SetFocus(fm.order[len(fm.order)-1])
	fm.Hide("form_test")
	fm.Show("form_test")
	if app.app.GetFocus() != first {
		t.Fatal("reopen did not reset focus to the first field")
	}
}

func TestFormModalTabCyclesFieldsThenButtonsAndWraps(t *testing.T) {
	app := newUXTestApp()
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	fm.AddTextArea("Body", "", 5)
	fm.AddButtons(FormButton{Label: "OK"}, FormButton{Label: "Cancel"})
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	for i := 1; i <= len(fm.order); i++ {
		capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
		want := fm.order[i%len(fm.order)]
		if app.app.GetFocus() != want {
			t.Fatalf("after %d tabs focus = %T, want order[%d]", i, app.app.GetFocus(), i%len(fm.order))
		}
	}
}

func TestFormModalEscCancelsAndCtrlEnterSubmits(t *testing.T) {
	app := newUXTestApp()
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	var canceled, submitted bool
	fm.SetOnCancel(func() { canceled = true })
	fm.SetOnSubmit(func() { submitted = true })
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !canceled {
		t.Fatal("Esc did not call onCancel")
	}
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl))
	if !submitted {
		t.Fatal("Ctrl+Enter did not call onSubmit")
	}
}

// TestFormModalConsecutivePickersShareARow verifies dropdowns added back to
// back pack into one two-row unit (labels above, values below) instead of
// stacking, and still tab in order.
func TestFormModalConsecutivePickersShareARow(t *testing.T) {
	app := newUXTestApp()
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	a := fm.AddPicker("Assignee", []string{"Unassigned"}, 0, nil)
	c := fm.AddPicker("Cycle", []string{"No cycle"}, 0, nil)
	p := fm.AddPicker("Priority", []string{"Normal"}, 0, nil)
	if len(fm.rows) != 2 {
		t.Fatalf("rows = %d, want 2 (input row + one shared picker row)", len(fm.rows))
	}
	if h := fm.rows[1].height; h != 2 {
		t.Fatalf("picker row height = %d, want 2", h)
	}
	for i, dd := range []tview.Primitive{a, c, p} {
		if fm.order[1+i] != dd {
			t.Fatalf("tab order position %d is not picker %d", 1+i, i)
		}
	}
}

// TestFormModalEscClosesOpenDropdownBeforeCanceling preserves the old
// closeOpenDropdown behavior: Esc with an open dropdown closes the dropdown,
// not the modal.
func TestFormModalEscClosesOpenDropdownBeforeCanceling(t *testing.T) {
	app := newUXTestApp()
	fm := NewFormModal(app, "Test")
	dd := fm.AddPicker("Priority", []string{"Normal", "High"}, 0, nil)
	var canceled bool
	fm.SetOnCancel(func() { canceled = true })
	fm.Show("form_test")

	handler := dd.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.app.SetFocus(p) })
	if !dd.IsOpen() {
		t.Fatal("dropdown did not open")
	}
	fm.frame.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if canceled {
		t.Fatal("Esc canceled the modal while a dropdown was open")
	}
	if dd.IsOpen() {
		t.Fatal("Esc did not close the open dropdown")
	}
}

// TestFormModalHeightFitsContentAndClampsToScreen pins the sizing math: the
// modal fits its content, and when the screen is short the flexible textarea
// row shrinks instead of clipping fields off the bottom.
func TestFormModalHeightFitsContentAndClampsToScreen(t *testing.T) {
	app := newUXTestApp()
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")       // 4 rows
	fm.AddTextArea("Body", "", 10) // 13 rows, flexible (min 6)
	fm.AddButtons(FormButton{Label: "OK"})

	pad := app.density.ModalPadding
	chrome := 1 + 1 + 1 + 2 + pad.Top + pad.Bottom // blank + buttons + hint + border + padding
	if got, want := fm.contentHeight(100), 4+13+chrome; got != want {
		t.Fatalf("unclamped height = %d, want %d", got, want)
	}
	if got := fm.contentHeight(20); got != 16 {
		t.Fatalf("clamped height = %d, want 16", got)
	}
}
