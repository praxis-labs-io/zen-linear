package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestFormModalShowResetsFocusToFirstField guards the whole bug class where
// reopening a modal leaves keyboard focus on whatever was focused last
// (tview forms remember their last-focused item across shows).
func TestFormModalShowResetsFocusToFirstField(t *testing.T) {
	app := newUXTestApp(t)
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
	app := newUXTestApp(t)
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
	app := newUXTestApp(t)
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
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	a := fm.AddPicker("Assignee", []string{"Unassigned"}, 0, nil)
	c := fm.AddPicker("Cycle", []string{"No cycle"}, 0, nil)
	p := fm.AddPicker("Priority", []string{"Normal"}, 0, nil)
	if len(fm.rows) != 2 {
		t.Fatalf("rows = %d, want 2 (input row + one shared picker row)", len(fm.rows))
	}
	if h := fm.rows[1].height; h != 4 {
		t.Fatalf("picker row height = %d, want 4 (label + framed value)", h)
	}
	for i, picker := range []*FormPicker{a, c, p} {
		if fm.order[1+i] != picker.view {
			t.Fatalf("tab order position %d is not picker %d", 1+i, i)
		}
	}
}

// TestFormModalEscClosesTheOpenMenuBeforeCanceling: Esc with a menu open
// closes the menu, not the modal.
func TestFormModalEscClosesTheOpenMenuBeforeCanceling(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	picker := fm.AddPicker("Priority", []string{"Normal", "High"}, 0, nil)
	var canceled bool
	fm.SetOnCancel(func() { canceled = true })
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if !picker.IsOpen() {
		t.Fatal("Enter did not open the menu")
	}
	capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if canceled {
		t.Fatal("Esc canceled the modal while the menu was open")
	}
	if picker.IsOpen() {
		t.Fatal("Esc did not close the menu")
	}
	capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !canceled {
		t.Fatal("Esc with no menu open did not cancel the modal")
	}
}

// TestFormModalMenuScrollsWithinItsCap covers the reason the form owns the
// menu: tview's DropDown grows its menu to the option count.
func TestFormModalMenuScrollsWithinItsCap(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	options := make([]string, 20)
	for i := range options {
		options[i] = string(rune('a' + i))
	}
	picker := fm.AddPicker("Project", options, 0, nil)
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	for i := 0; i < len(options)+5; i++ {
		capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	}
	if got := fm.menu.GetCurrentItem(); got != len(options)-1 {
		t.Fatalf("menu cursor = %d, want it stopped at the last option %d", got, len(options)-1)
	}
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if picker.IsOpen() {
		t.Fatal("Enter did not close the menu")
	}
	if index, text := picker.GetCurrentOption(); index != len(options)-1 || text != options[len(options)-1] {
		t.Fatalf("selection = %d/%q, want the last option", index, text)
	}
}

// TestFormModalArrowsStayWithTheFocusedField keeps navigation on Tab alone.
// Arrows moving focus is a footgun: it steals the keys a text cursor, an open
// dropdown, and a list each need.
func TestFormModalArrowsStayWithTheFocusedField(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	first := fm.AddInput("Title", "")
	fm.AddTextArea("Body", "", 5)
	fm.AddPicker("Priority", []string{"Normal", "High"}, 0, nil)
	fm.AddButtons(FormButton{Label: "OK"})
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	for _, key := range []tcell.Key{tcell.KeyDown, tcell.KeyUp, tcell.KeyLeft, tcell.KeyRight} {
		event := tcell.NewEventKey(key, 0, tcell.ModNone)
		if got := capture(event); got != event {
			t.Fatalf("key %v was swallowed by the form, want it passed to the field", key)
		}
		if app.app.GetFocus() != first {
			t.Fatalf("key %v moved focus off the first field", key)
		}
	}
}

// TestFormModalEndRowBreaksThePack guards the layout of a form with more
// packed fields than fit one row: without EndRow they all pack into one.
func TestFormModalEndRowBreaksThePack(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddPicker("Status", []string{"Todo"}, 0, nil)
	fm.AddPicker("Assignee", []string{"Unassigned"}, 0, nil)
	fm.AddPicker("Priority", []string{"Normal"}, 0, nil)
	fm.EndRow()
	fm.AddPicker("Project", []string{"No project"}, 0, nil)
	fm.AddPackedInput("Estimate", "")

	if len(fm.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(fm.rows))
	}
	if fm.rows[0].columns != 3 || fm.rows[1].columns != 2 {
		t.Fatalf("columns = %d and %d, want 3 and 2", fm.rows[0].columns, fm.rows[1].columns)
	}
	if len(fm.order) != 5 {
		t.Fatalf("tab stops = %d, want 5", len(fm.order))
	}
}

// TestFormModalMultiSelectTogglesAndReadsBackSorted covers the inline
// multi-select: Space ticks the highlighted row, Tab still leaves the field.
func TestFormModalMultiSelectTogglesAndReadsBackSorted(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	ms := fm.AddMultiSelect("Labels", 4)
	fm.AddInput("Title", "")
	ms.SetItems([]MultiSelectItem{
		{ID: "label-chore", Label: "Chore"},
		{ID: "label-bug", Label: "Bug"},
	}, nil)
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if first, _ := ms.list.GetItemText(0); first != "(x) Chore" {
		t.Fatalf("first row = %q, want it ticked", first)
	}
	ms.list.SetCurrentItem(1)
	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if got := ms.SelectedIDs(); len(got) != 2 || got[0] != "label-bug" || got[1] != "label-chore" {
		t.Fatalf("SelectedIDs() = %v, want both ids sorted", got)
	}

	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if got := ms.SelectedIDs(); len(got) != 1 || got[0] != "label-chore" {
		t.Fatalf("SelectedIDs() = %v, want the second row untoggled", got)
	}
	capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if app.app.GetFocus() == ms.list {
		t.Fatal("Tab did not leave the multi-select")
	}
}

// TestFormModalMultiSelectKeepsSelectionAcrossSetItems covers the async fill:
// the options arrive after the form has already been told what is ticked.
func TestFormModalMultiSelectKeepsSelectionAcrossSetItems(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	ms := fm.AddMultiSelect("Labels", 4)
	ms.SetPlaceholder("Loading...")
	ms.SetItems(nil, []string{"label-bug"})

	if first, _ := ms.list.GetItemText(0); first != "Loading..." {
		t.Fatalf("empty list row = %q, want the placeholder", first)
	}
	ms.SetItems([]MultiSelectItem{{ID: "label-bug", Label: "Bug"}, {ID: "label-chore", Label: "Chore"}}, []string{"label-bug"})

	if first, _ := ms.list.GetItemText(0); first != "(x) Bug" {
		t.Fatalf("first row = %q, want the prior selection ticked", first)
	}
}

// TestFormModalHiddenRowTakesNoHeight covers the parent line the issue form
// hides outside sub-issue create.
func TestFormModalHiddenRowTakesNoHeight(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddStatic("Parent: ZNL-1")
	fm.AddInput("Title", "")

	full := fm.contentHeight(100)
	fm.SetRowHidden(0, true)
	if got := fm.contentHeight(100); got != full-1 {
		t.Fatalf("height with the static row hidden = %d, want %d", got, full-1)
	}
	if heights := fm.rowHeights(100); heights[0] != 0 {
		t.Fatalf("hidden row height = %d, want 0", heights[0])
	}
}

// TestFormModalWindowClipsRatherThanDropsTheFocusedRow guards the scroll
// behavior on a short screen: a field taller than the window used to get zero
// height and vanish while it held focus.
func TestFormModalWindowClipsRatherThanDropsTheFocusedRow(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	fm.AddTextArea("Body", "", 10)
	fm.AddInput("Footer", "")

	// A window smaller than the textarea row it starts on.
	heights := fm.rowHeights(100)
	fm.scrollTop = 1
	shown := fm.applyRowWindow(heights, 4)

	if shown[1] != 4 {
		t.Fatalf("focused row height = %d, want the window's 4 lines rather than 0", shown[1])
	}
	if shown[0] != 0 || shown[2] != 0 {
		t.Fatalf("off-window rows = %d and %d, want 0", shown[0], shown[2])
	}
	if !fm.scrollBelow {
		t.Fatal("scrollBelow is false with a clipped row, so no marker would be drawn")
	}
	if !fm.scrollAbove {
		t.Fatal("scrollAbove is false with rows scrolled off the top")
	}
}

// TestFormModalHeightFitsContentAndClampsToScreen pins the sizing math: the
// modal fits its content, and when the screen is short the flexible textarea
// row shrinks instead of clipping fields off the bottom.
func TestFormModalHeightFitsContentAndClampsToScreen(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")       // 4 rows
	fm.AddTextArea("Body", "", 10) // 13 rows, flexible (min 6)
	fm.AddButtons(FormButton{Label: "OK"})

	chrome := 1 + 1 + 1 + 1 + 2 // blank + buttons + gap + hint + border
	if got, want := fm.contentHeight(100), 4+13+chrome; got != want {
		t.Fatalf("unclamped height = %d, want %d", got, want)
	}
	if got := fm.contentHeight(20); got != 16 {
		t.Fatalf("clamped height = %d, want 16", got)
	}
}

// TestPickerMenuRectPlacesTheMenuAgainstItsField pins the geometry the draw
// path can't be asserted on: same edges as the field, capped height with a
// border, dropping up when the screen ends first.
func TestPickerMenuRectPlacesTheMenuAgainstItsField(t *testing.T) {
	cases := []struct {
		name                     string
		fieldY, options, screenH int
		wantY, wantHeight        int
		wantFits                 bool
	}{
		{name: "below the field", fieldY: 4, options: 3, screenH: 40, wantY: 7, wantHeight: 5, wantFits: true},
		{name: "capped at eight rows", fieldY: 4, options: 40, screenH: 40, wantY: 7, wantHeight: 10, wantFits: true},
		{name: "drops up with no room below", fieldY: 25, options: 40, screenH: 30, wantY: 15, wantHeight: 10, wantFits: true},
		{name: "no options", fieldY: 4, options: 0, screenH: 40, wantFits: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, y, width, height, fits := pickerMenuRect(6, tc.fieldY, 20, 3, tc.options, tc.screenH)
			if fits != tc.wantFits {
				t.Fatalf("fits = %v, want %v", fits, tc.wantFits)
			}
			if !fits {
				return
			}
			if x != 6 || width != 20 {
				t.Fatalf("menu x/width = %d/%d, want the field's 6/20", x, width)
			}
			if y != tc.wantY || height != tc.wantHeight {
				t.Fatalf("menu y/height = %d/%d, want %d/%d", y, height, tc.wantY, tc.wantHeight)
			}
		})
	}
}
