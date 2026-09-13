package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/rivo/tview"
)

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

func TestFormModalMultiSelectTogglesAndReadsBackSorted(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	ms, _ := fm.AddSplitRow("Labels", 4, []string{"Due date"})
	fm.AddInput("Title", "")
	ms.SetItems([]MultiSelectItem{
		{ID: "label-chore", Label: "Chore"},
		{ID: "label-bug", Label: "Bug"},
	}, nil)
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if first, _ := ms.list.GetItemText(0); markOf(first) != '◼' {
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

func TestFormModalMultiSelectKeepsSelectionAcrossSetItems(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	ms, _ := fm.AddSplitRow("Labels", 4, []string{"Due date"})
	ms.SetPlaceholder("Loading...")
	ms.SetItems(nil, []string{"label-bug"})

	if first, _ := ms.list.GetItemText(0); first != "Loading..." {
		t.Fatalf("empty list row = %q, want the placeholder", first)
	}
	ms.SetItems([]MultiSelectItem{{ID: "label-bug", Label: "Bug"}, {ID: "label-chore", Label: "Chore"}}, []string{"label-bug"})

	if first, _ := ms.list.GetItemText(0); markOf(first) != '◼' {
		t.Fatalf("first row = %q, want the prior selection ticked", first)
	}
}

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

func TestFormModalWindowClipsRatherThanDropsTheFocusedRow(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	fm.AddTextArea("Body", "", 10)
	fm.AddInput("Footer", "")

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

func TestFormModalHeightFitsContentAndClampsToScreen(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	fm.AddTextArea("Body", "", 10)
	fm.AddButtons(FormButton{Label: "OK"})

	chrome := 1 + 1 + 1 + 1 + 2
	if got, want := fm.contentHeight(100), 4+13+chrome; got != want {
		t.Fatalf("unclamped height = %d, want %d", got, want)
	}
	if got := fm.contentHeight(20); got != 16 {
		t.Fatalf("clamped height = %d, want 16", got)
	}
}

func TestPickerMenuRectPlacesTheMenuAgainstItsField(t *testing.T) {
	const fieldX, fieldWidth, fieldHeight = 6, 20, 3
	cases := []struct {
		name                     string
		fieldY, options, longest int
		screenH                  int
		wantY, wantHeight        int
		wantWidth                int
		wantFits                 bool
	}{
		{name: "over the field's bottom border", fieldY: 4, options: 3, longest: 8, screenH: 40, wantY: 6, wantHeight: 5, wantWidth: 20, wantFits: true},
		{name: "capped at eight rows", fieldY: 4, options: 40, longest: 8, screenH: 40, wantY: 6, wantHeight: 10, wantWidth: 20, wantFits: true},
		{name: "widens for a long option", fieldY: 4, options: 2, longest: 30, screenH: 40, wantY: 6, wantHeight: 4, wantWidth: 32, wantFits: true},
		{name: "drops up with no room below", fieldY: 25, options: 40, longest: 8, screenH: 30, wantY: 16, wantHeight: 10, wantWidth: 20, wantFits: true},
		{name: "clips to the roomier side", fieldY: 3, options: 40, longest: 8, screenH: 12, wantY: 5, wantHeight: 7, wantWidth: 20, wantFits: true},
		{name: "no options", fieldY: 4, options: 0, longest: 0, screenH: 40, wantFits: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, y, width, height, fits := pickerMenuRect(fieldX, tc.fieldY, fieldWidth, fieldHeight, tc.options, tc.longest, 100, tc.screenH)
			if fits != tc.wantFits {
				t.Fatalf("fits = %v, want %v", fits, tc.wantFits)
			}
			if !fits {
				return
			}
			if x != fieldX || width != tc.wantWidth {
				t.Fatalf("menu x/width = %d/%d, want %d/%d", x, width, fieldX, tc.wantWidth)
			}
			if y != tc.wantY || height != tc.wantHeight {
				t.Fatalf("menu y/height = %d/%d, want %d/%d", y, height, tc.wantY, tc.wantHeight)
			}
		})
	}
}

func TestPickerMenuRectShiftsLeftAtTheScreenEdge(t *testing.T) {
	x, _, width, _, fits := pickerMenuRect(70, 4, 20, 3, 3, 28, 80, 40)
	if !fits {
		t.Fatal("menu did not fit")
	}
	if width != 30 {
		t.Fatalf("width = %d, want 30 for the longest option", width)
	}
	if x != 50 || x+width > 80 {
		t.Fatalf("x = %d with width %d, want it shifted back onto an 80-column screen", x, width)
	}
}

func TestFormModalSplitRowColumnsTheFields(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	fm.AddInput("Title", "")
	multi, inputs := fm.AddSplitRow("Labels", 5, []string{"Due date", "Estimate"})

	if len(fm.rows) != 2 {
		t.Fatalf("rows = %d, want the input row and one split row", len(fm.rows))
	}
	if len(inputs) != 2 {
		t.Fatalf("side fields = %d, want 2", len(inputs))
	}
	if fm.rows[1].height != 8 {
		t.Fatalf("split row height = %d, want 8 (two stacked fields)", fm.rows[1].height)
	}
	want := []tview.Primitive{fm.order[0], multi.list, inputs[0], inputs[1]}
	for i, primitive := range want {
		if fm.order[i] != primitive {
			t.Fatalf("tab stop %d is not the expected field", i)
		}
	}
}

func TestFormModalMenuClosesWhenFocusLeavesIt(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	picker := fm.AddPicker("Status", []string{"Todo", "Doing"}, 0, nil)
	other := fm.AddInput("Title", "")
	fm.Show("form_test")

	capture := fm.frame.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if !picker.IsOpen() {
		t.Fatal("Enter did not open the menu")
	}

	app.app.SetFocus(other)

	if picker.IsOpen() {
		t.Fatal("the menu stayed open after focus moved to another field")
	}
	capture(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	if fm.openPicker != nil {
		t.Fatal("keys are still routed into the menu")
	}
}

func TestScrolledOffRowsDoNotPaintOverTheChrome(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 80, 24)

	fm := NewFormModal(app, "Test")
	labels := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel"}
	for _, label := range labels {
		fm.AddInput(label, "")
	}
	fm.AddPicker("India", []string{"one"}, 0, nil)
	fm.AddPicker("Juliett", []string{"one"}, 0, nil)
	fm.AddButtons(FormButton{Label: "Save"}, FormButton{Label: "Cancel"})
	fm.SetHint("Esc cancel")
	fm.Show("form_test")

	lines := drawPrimitiveAt(t, fm.Root(), 80, 24)
	screen := strings.Join(lines, "\n")

	if !strings.Contains(screen, "Save") || !strings.Contains(screen, "Cancel") {
		t.Fatalf("the button row is not on screen:\n%s", screen)
	}
	if !strings.Contains(screen, "Esc cancel") {
		t.Fatalf("the hint line is not on screen:\n%s", screen)
	}

	last := -1
	for i, label := range labels {
		if strings.Contains(screen, strings.ToUpper(label)) {
			last = i
		}
	}
	if last < 0 {
		t.Fatalf("no field label drew at all:\n%s", screen)
	}
	for i, label := range labels {
		if i <= last {
			continue
		}
		if strings.Contains(screen, strings.ToUpper(label)) {
			t.Fatalf("%q is off the window but painted anyway:\n%s", label, screen)
		}
	}
	for _, label := range []string{"INDIA", "JULIETT"} {
		if strings.Contains(screen, label) {
			t.Fatalf("packed row %q is off the window but painted anyway:\n%s", label, screen)
		}
	}
}

func TestPackedLabelsTruncateRatherThanWrap(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 50, 30)

	fm := NewFormModal(app, "Test")
	fm.AddPicker("Agent provider", []string{"one"}, 0, nil)
	fm.AddPicker("Agent sandbox", []string{"one"}, 0, nil)
	fm.AddPicker("Agent model", []string{"one"}, 0, nil)
	fm.Show("form_test")

	lines := drawPrimitiveAt(t, fm.Root(), 50, 30)
	drawn := map[string]bool{}
	for _, line := range lines {
		for _, label := range regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), -1) {
			if strings.HasPrefix(label, "AGENT") {
				drawn[label] = true
			}
		}
	}
	if len(drawn) != 3 {
		t.Fatalf("three labels drew %d distinct texts, so at least two read the same:\n%s",
			len(drawn), strings.Join(lines, "\n"))
	}
}

func TestPackedRowFoldsWhenColumnsGetNarrow(t *testing.T) {
	app := newUXTestApp(t)
	fm := NewFormModal(app, "Test")
	for _, label := range []string{"Timeout", "Page size", "Cache TTL", "Debounce"} {
		fm.AddPackedInput(label, "")
	}

	for _, tc := range []struct {
		screenW int
		want    int
	}{
		{120, formFieldRows},
		{50, formFieldRows * 2},
		{20, formFieldRows * 4},
	} {
		app.pages.SetRect(0, 0, tc.screenW, 40)
		if got := fm.rowHeights(40)[0]; got != tc.want {
			t.Fatalf("at %d columns the packed row is %d lines, want %d", tc.screenW, got, tc.want)
		}
	}
}

func TestAFoldedRowKeepsEveryFieldOnScreen(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 50, 40)

	fm := NewFormModal(app, "Test")
	labels := []string{"Timeout", "Page size", "Cache TTL", "Debounce"}
	for _, label := range labels {
		fm.AddPackedInput(label, "")
	}
	fm.Show("form_test")

	screen := strings.Join(drawPrimitiveAt(t, fm.Root(), 50, 40), "\n")
	for _, label := range labels {
		if !strings.Contains(screen, strings.ToUpper(label)) {
			t.Fatalf("%q is missing after the fold:\n%s", label, screen)
		}
	}
}

func TestSectionsLayOutOnlyTheOpenPage(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("First")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Second")
	fm.AddInput("Bravo", "")
	fm.Show("form_test")

	screen := strings.Join(drawPrimitiveAt(t, fm.Root(), 100, 40), "\n")
	if !strings.Contains(screen, "ALPHA") {
		t.Fatalf("the open section's field is missing:\n%s", screen)
	}
	if strings.Contains(screen, "BRAVO") {
		t.Fatalf("a closed section's field drew anyway:\n%s", screen)
	}

	fm.stepSection(1)
	screen = strings.Join(drawPrimitiveAt(t, fm.Root(), 100, 40), "\n")
	if !strings.Contains(screen, "BRAVO") {
		t.Fatalf("stepping to the second section did not open it:\n%s", screen)
	}
	if strings.Contains(screen, "ALPHA") {
		t.Fatalf("the first section stayed laid out:\n%s", screen)
	}
}

func TestTheRailGivesUpItsColumnOnANarrowPanel(t *testing.T) {
	app := newUXTestApp(t)

	fm := NewFormModal(app, "Test")
	fm.SetMaxWidth(110)
	fm.BeginSection("Appearance")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Network & logging")
	fm.AddInput("Bravo", "")

	app.pages.SetRect(0, 0, 110, 40)
	if !fm.railIsVertical() {
		t.Fatal("a wide panel did not give the rail its own column")
	}
	fm.Show("form_test")
	wide := strings.Join(drawPrimitiveAt(t, fm.Root(), 110, 40), "\n")
	if !strings.Contains(wide, "Network & logging") {
		t.Fatalf("the rail did not list every section:\n%s", wide)
	}

	app.pages.SetRect(0, 0, 56, 40)
	if fm.railIsVertical() {
		t.Fatal("a narrow panel kept the rail's column")
	}
	narrow := strings.Join(drawPrimitiveAt(t, fm.Root(), 56, 40), "\n")
	if !strings.Contains(narrow, "‹ Appearance ›") {
		t.Fatalf("the narrow rail does not name the open section:\n%s", narrow)
	}
	if !strings.Contains(narrow, "ALPHA") {
		t.Fatalf("the open section's field is missing on a narrow panel:\n%s", narrow)
	}
}

func TestAnEmbeddedFormDrawsItsFields(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)
	app.promptTemplatesModal.Show(nil, func([]config.AgentPromptTemplate) error { return nil })

	screen := strings.Join(drawPrimitiveAt(t, app.promptTemplatesModal.modal, 100, 40), "\n")
	for _, label := range []string{"NAME", "PROMPT"} {
		if !strings.Contains(screen, label) {
			t.Fatalf("the embedded form did not draw %q:\n%s", label, screen)
		}
	}
}

func TestTabSkipsFieldsInAClosedSection(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("First")
	alpha := fm.AddInput("Alpha", "")
	fm.BeginSection("Second")
	bravo := fm.AddInput("Bravo", "")
	fm.AddButtons(FormButton{Label: "Save"})
	fm.Show("form_test")

	fm.stepSection(1)
	fm.focusStep(1)
	if got := app.app.GetFocus(); got == alpha {
		t.Fatal("Tab reached a field in the closed section, which is not mounted")
	} else if got != bravo {
		t.Fatalf("Tab reached %T, want the open section's own field", got)
	}

	fm.focusStep(-1)
	if got := app.app.GetFocus(); got == alpha {
		t.Fatal("Backtab reached a field in the closed section")
	}
}

func TestTheRailIsAPaneOfItsOwn(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	canceled := false
	fm := NewFormModal(app, "Test")
	fm.SetOnCancel(func() { canceled = true })
	fm.BeginSection("First")
	alpha := fm.AddInput("Alpha", "")
	fm.BeginSection("Second")
	bravo := fm.AddInput("Bravo", "")
	fm.Show("form_test")

	if !fm.railHasFocus() {
		t.Fatal("a sectioned form did not open on its rail")
	}
	fm.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if app.app.GetFocus() != alpha {
		t.Fatal("Enter on the rail did not cross into the open section's first field")
	}
	if canceled {
		t.Fatal("Enter on the rail closed the modal")
	}

	fm.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !fm.railHasFocus() {
		t.Fatal("Esc in a field did not come back to the rail")
	}
	if canceled {
		t.Fatal("Esc in a field closed the modal instead of backing out one level")
	}

	fm.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	fm.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone))
	if app.app.GetFocus() != bravo {
		t.Fatal("stepping the rail and pressing l did not reach the second section's field")
	}

	fm.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	fm.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !canceled {
		t.Fatal("Esc on the rail did not close the modal")
	}
}

func TestSteppingSectionsKeepsThePanelOneSize(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("Short")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Long")
	for _, label := range []string{"Bravo", "Charlie", "Delta"} {
		fm.AddInput(label, "")
	}
	fm.Show("form_test")

	short := fm.contentHeight(40)
	fm.stepSection(1)
	if long := fm.contentHeight(40); long != short {
		t.Fatalf("the panel is %d lines on the short section and %d on the long one", short, long)
	}
}

func TestTheRailMarksWhichPaneHasTheKeyboard(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("First")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Second")
	fm.AddInput("Bravo", "")
	fm.Show("form_test")

	if !fm.railHasFocus() {
		t.Fatal("the form did not open on the section list")
	}
	drawPrimitiveAt(t, fm.Root(), 100, 40)
	focused := fm.sectionRail.GetText(false)
	if !strings.Contains(focused, app.themeTags.Selection) {
		t.Fatalf("the open section is not on the cursor line while the list has the keyboard: %q", focused)
	}

	fm.enterSection()
	drawPrimitiveAt(t, fm.Root(), 100, 40)
	blurred := fm.sectionRail.GetText(false)
	if blurred == focused {
		t.Fatalf("the section list looks the same with and without the keyboard: %q", blurred)
	}
	if strings.Contains(blurred, app.themeTags.Selection) {
		t.Fatalf("the cursor line stayed on the list after the fields took the keyboard: %q", blurred)
	}
}

func TestAClickOnTheFieldsDoesNotPickASection(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 110, 40)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("First")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Second")
	fm.AddInput("Bravo", "")
	fm.Show("form_test")
	drawPrimitiveAt(t, fm.Root(), 110, 40)

	railX, railY, railWidth, _ := fm.sectionRail.GetRect()
	handler := fm.Root().MouseHandler()

	press := tcell.NewEventMouse(railX+railWidth+20, railY+railTopPad+1, tcell.Button1, tcell.ModNone)
	handler(tview.MouseLeftDown, press, func(tview.Primitive) {})
	if fm.activeSection != 0 {
		t.Fatalf("a click on the fields opened section %d", fm.activeSection)
	}

	onRail := tcell.NewEventMouse(railX+1, railY+railTopPad+1, tcell.Button1, tcell.ModNone)
	handler(tview.MouseLeftDown, onRail, func(tview.Primitive) {})
	if fm.activeSection != 1 {
		t.Fatalf("a click on the rail's second row opened section %d", fm.activeSection)
	}
}

func TestTheNavKeepsFocusWhenAPageIsAddedOrRemoved(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 110, 40)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("First")
	alpha := fm.AddInput("Alpha", "")
	fm.BeginSection("Second")
	fm.AddInput("Bravo", "")
	fm.Show("form_test")

	if !fm.railHasFocus() {
		t.Fatal("the form did not open on the section list")
	}

	app.pages.AddPage("decoy", tview.NewBox(), true, false)
	app.pages.RemovePage("decoy")
	if got := app.app.GetFocus(); got == alpha {
		t.Fatal("a page add and remove moved the keyboard onto the first field")
	}
	if !fm.railHasFocus() {
		t.Fatalf("the section list lost the keyboard to %T", app.app.GetFocus())
	}
}

func TestTheSidebarHoldsTheButtonsAndTheRulesMeet(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 34)

	fm := NewFormModal(app, "Test")
	fm.BeginSection("Appearance")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Logging")
	fm.AddInput("Bravo", "")
	fm.AddButtons(FormButton{Label: "Save"}, FormButton{Label: "Cancel"})
	fm.SetHint("Esc cancel")
	fm.Show("form_test")

	lines := drawPrimitiveAt(t, fm.Root(), 100, 34)
	screen := strings.Join(lines, "\n")

	hint, footer, save := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "Esc cancel"):
			hint = i
		case strings.Contains(line, "├") && strings.Contains(line, "┴"):
			footer = i
		case strings.Contains(line, "Save"):
			save = i
		}
	}
	if footer < 0 {
		t.Fatalf("the column rule does not close into the footer rule in a tee:\n%s", screen)
	}
	if hint < 0 || footer != hint-1 {
		t.Fatalf("the rule does not sit directly above the hint:\n%s", screen)
	}
	if save < 0 || save >= footer {
		t.Fatalf("the buttons are not inside the sidebar, above the rule:\n%s", screen)
	}
	top := -1
	for i, line := range lines {
		if strings.Contains(line, "┌") {
			top = i
			break
		}
	}
	if top < 0 || !strings.Contains(lines[top], "┬") {
		t.Fatalf("the column rule does not tee into the top border:\n%s", screen)
	}

	column := strings.Index(lines[footer], "┴")
	if got := strings.Index(lines[save], "Save"); got < 0 || got > column {
		t.Fatalf("Save is at column %d, right of the rule at %d:\n%s", got, column, screen)
	}
}

func TestASectionedFormSurvivesCrossingTheRailThreshold(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	fm := NewFormModal(app, "Test")
	fm.SetMaxWidth(82)
	fm.BeginSection("Appearance")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Logging")
	fm.AddInput("Bravo", "")
	fm.AddButtons(FormButton{Label: "Save"}, FormButton{Label: "Cancel"})
	fm.SetHint("Esc cancel")
	fm.Show("form_test")

	for _, width := range []int{100, 56, 100, 56, 100} {
		app.pages.SetRect(0, 0, width, 40)
		lines := drawPrimitiveAt(t, fm.Root(), width, 40)
		screen := strings.Join(lines, "\n")

		if strings.Count(screen, "Save") != 1 {
			t.Fatalf("at %d columns Save drew %d times, want once:\n%s",
				width, strings.Count(screen, "Save"), screen)
		}
		if strings.Count(screen, "Cancel") != 1 {
			t.Fatalf("at %d columns Cancel drew %d times, want once:\n%s",
				width, strings.Count(screen, "Cancel"), screen)
		}
		if !strings.Contains(screen, "Esc cancel") {
			t.Fatalf("at %d columns the hint is gone:\n%s", width, screen)
		}
		if !strings.Contains(screen, "ALPHA") {
			t.Fatalf("at %d columns the open section's field is gone:\n%s", width, screen)
		}
	}
}

func TestTheColumnRuleLeavesTheContextRowAlone(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 40)

	notice := "theme and log level are set by the environment"
	fm := NewFormModal(app, "Test")
	fm.BeginSection("Appearance")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Logging")
	fm.AddInput("Bravo", "")
	fm.SetContext(notice)
	fm.Show("form_test")

	for _, line := range drawPrimitiveAt(t, fm.Root(), 100, 40) {
		if strings.Contains(line, "environment") {
			if !strings.Contains(line, notice) {
				t.Fatalf("the column rule struck through the context row: %q", line)
			}
			return
		}
	}
	t.Fatal("the context row did not draw")
}

func TestTheArrowsWalkTheWholeSidebar(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 100, 34)

	saved := false
	fm := NewFormModal(app, "Test")
	fm.BeginSection("Appearance")
	fm.AddInput("Alpha", "")
	fm.BeginSection("Logging")
	fm.AddInput("Bravo", "")
	fm.AddButtons(
		FormButton{Label: "Save", OnPress: func() { saved = true }},
		FormButton{Label: "Cancel"},
	)
	fm.Show("form_test")

	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)

	fm.HandleKey(down)
	if fm.activeSection != 1 {
		t.Fatalf("first Down left the section at %d", fm.activeSection)
	}
	fm.HandleKey(down)
	if got := fm.focusedButton(); got != 0 {
		t.Fatalf("Down off the last section reached button %d, want Save", got)
	}
	fm.HandleKey(down)
	if got := fm.focusedButton(); got != 1 {
		t.Fatalf("Down off Save reached button %d, want Cancel", got)
	}

	fm.HandleKey(up)
	if got := fm.focusedButton(); got != 0 {
		t.Fatalf("Up off Cancel reached button %d, want Save", got)
	}
	fm.HandleKey(up)
	if !fm.railHasFocus() {
		t.Fatal("Up off the first button did not go back to the section list")
	}
	if fm.activeSection != 1 {
		t.Fatalf("coming back off the buttons opened section %d, want the last one", fm.activeSection)
	}

	fm.HandleKey(down)
	if got := fm.focusedButton(); got != 0 {
		t.Fatalf("Down from the last section reached button %d, want Save", got)
	}
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	if left := fm.HandleKey(enter); left != nil {
		if handler := app.app.GetFocus().InputHandler(); handler != nil {
			handler(left, func(tview.Primitive) {})
		}
	}
	if !saved {
		t.Fatal("Enter on Save did not press it")
	}
}
