package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// appOnScreen builds an app drawn on a terminal of the given size. The modals
// size themselves against the screen, and the screen is only known once
// something has been drawn on it.
func appOnScreen(t *testing.T, width, height int) *App {
	t.Helper()
	app := newUXTestApp(t)
	screen := tcell.NewSimulationScreen("UTF-8")
	app.app.SetScreen(screen)
	screen.SetSize(width, height)
	app.app.SetRoot(app.pages, true)
	app.app.ForceDraw()
	return app
}

// listPanelRect is where a list modal's bordered panel was drawn, worked back
// from the list inside it: the panel is the list plus a gutter and a border
// each side.
func listPanelRect(t *testing.T, list *tview.List) (x, y, width int) {
	t.Helper()
	listX, listY, listWidth, _ := list.GetRect()
	if listWidth == 0 {
		t.Fatal("the modal was never drawn")
	}
	return listX - modalGutter - 1, listY, listWidth + 2*(modalGutter+1)
}

// rowsOf is where a primitive was drawn vertically: its top row and how many
// rows it took.
func rowsOf(p tview.Primitive) (y, height int) {
	_, y, _, height = p.GetRect()
	return y, height
}

func pickerItems(n int) []PickerItem {
	items := make([]PickerItem, n)
	for i := range items {
		items[i] = PickerItem{ID: string(rune('a' + i)), Label: "Option"}
	}
	return items
}

// TestListModalsFitASmallScreen pins both panels inside the terminal. Laid out
// at a fixed 50x15 and 60x20 they were drawn from a negative origin, which took
// the footer off a short screen and the first columns of every row off a narrow
// one.
func TestListModalsFitASmallScreen(t *testing.T) {
	sizes := []struct{ width, height int }{
		{100, 12},
		{44, 14},
		{40, 10},
		{100, 30},
	}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("picker %dx%d", size.width, size.height), func(t *testing.T) {
			app := appOnScreen(t, size.width, size.height)
			app.pickerModal.Show("Set Priority", pickerItems(12), func(PickerItem) {})
			app.app.ForceDraw()
			assertPanelOnScreen(t, app.pickerModal.list, app.pickerModal.hintView, size.width, size.height)
		})
		t.Run(fmt.Sprintf("multi-select %dx%d", size.width, size.height), func(t *testing.T) {
			app := appOnScreen(t, size.width, size.height)
			app.multiSelectModal.Show("Filter Labels", multiSelectItems(12), nil, func([]string) {})
			app.app.ForceDraw()
			assertPanelOnScreen(t, app.multiSelectModal.list, app.multiSelectModal.hintView, size.width, size.height)
		})
	}
}

// assertPanelOnScreen checks the panel's own edges, worked back from the two
// rows that sit against them: the list is a gutter and a border in from each
// side, and the hint is the last row above the bottom border.
func assertPanelOnScreen(t *testing.T, list *tview.List, hint *tview.TextView, screenW, screenH int) {
	t.Helper()
	x, listY, width := listPanelRect(t, list)
	hintY, _ := rowsOf(hint)

	if x < 0 || x+width > screenW {
		t.Errorf("panel spans columns %d..%d on a %d-column screen", x, x+width, screenW)
	}
	// A border row above the list, and one below the hint.
	if listY < 1 {
		t.Errorf("the list starts at row %d, leaving no room for the top border", listY)
	}
	if bottom := hintY + 1; bottom >= screenH {
		t.Errorf("the bottom border lands on row %d of a %d-row screen", bottom, screenH)
	}
}

// TestPickerHeightTracksItsOptions is the fixed-height fix: a two-option picker
// cost fifteen rows whatever it held.
func TestPickerHeightTracksItsOptions(t *testing.T) {
	short := appOnScreen(t, 100, 30)
	short.pickerModal.Show("Group Issues By", pickerItems(2), func(PickerItem) {})
	short.app.ForceDraw()
	_, shortRows := rowsOf(short.pickerModal.list)

	long := appOnScreen(t, 100, 30)
	long.pickerModal.Show("Group Issues By", pickerItems(10), func(PickerItem) {})
	long.app.ForceDraw()
	_, longRows := rowsOf(long.pickerModal.list)

	if shortRows != 2 {
		t.Errorf("a two-option picker drew %d rows, want 2", shortRows)
	}
	if longRows != 10 {
		t.Errorf("a ten-option picker drew %d rows, want 10", longRows)
	}
}

// TestPickerCapsItsHeight keeps a long list scrolling rather than filling the
// terminal.
func TestPickerCapsItsHeight(t *testing.T) {
	app := appOnScreen(t, 100, 40)
	app.pickerModal.Show("Set Status", pickerItems(30), func(PickerItem) {})
	app.app.ForceDraw()
	if _, rows := rowsOf(app.pickerModal.list); rows != pickerMaxVisibleRows {
		t.Errorf("a thirty-option picker drew %d rows, want the %d cap", rows, pickerMaxVisibleRows)
	}
}

// TestPickerNamesItselfOnTheBorderOnly is the duplicate-title fix: the title
// was a content row, and edit labels drew it twice.
func TestPickerNamesItselfOnTheBorderOnly(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	app.pickerModal.Show("Set Priority", pickerItems(3), func(PickerItem) {})
	for i := 0; i < app.pickerModal.list.GetItemCount(); i++ {
		if text, _ := app.pickerModal.list.GetItemText(i); text == "Set Priority" {
			t.Fatalf("row %d repeats the modal title", i)
		}
	}
}

// TestEmptyPickerSaysSoAndPicksNothing covers the missing empty state. An
// options-less picker used to draw a blank box that answered Enter.
func TestEmptyPickerSaysSoAndPicksNothing(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	chosen := false
	app.pickerModal.Show("Remove Relation", nil, func(PickerItem) { chosen = true })

	if got := app.pickerModal.list.GetItemCount(); got != 1 {
		t.Fatalf("drew %d rows, want one placeholder", got)
	}
	if text, _ := app.pickerModal.list.GetItemText(0); !strings.Contains(text, "No options") {
		t.Errorf("placeholder row = %q, want it to say there are none", text)
	}

	app.pickerModal.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	app.pickerModal.HandleKey(tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModNone))
	if chosen {
		t.Error("an empty picker reported a selection")
	}
	if !app.pages.HasPage("picker") {
		t.Error("Enter over the placeholder closed the picker")
	}
}

func multiSelectItems(n int) []MultiSelectItem {
	items := make([]MultiSelectItem, n)
	for i := range items {
		items[i] = MultiSelectItem{ID: string(rune('a' + i)), Label: "Option"}
	}
	return items
}

// TestEmptyMultiSelectSavesNothing is the other half of the empty state: an
// empty selection over real options is a choice, over none it is not.
func TestEmptyMultiSelectSavesNothing(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	saved := false
	app.multiSelectModal.Show("Filter Labels", nil, nil, func([]string) { saved = true })

	if text, _ := app.multiSelectModal.list.GetItemText(0); !strings.Contains(text, "No options") {
		t.Errorf("placeholder row = %q, want it to say there are none", text)
	}

	app.multiSelectModal.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	app.multiSelectModal.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if saved {
		t.Error("a multi-select with no options saved a selection")
	}
	if app.pages.HasPage("multi_select") {
		t.Error("Enter over the placeholder left the modal open")
	}
}

// TestMultiSelectRewritesOneRowPerToggle pins the arrow keys to moving the
// list's own cursor. Every press used to rebuild every row to redraw a
// hand-rolled "> " marker.
func TestMultiSelectRewritesOneRowPerToggle(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	app.multiSelectModal.Show("Filter Labels", []MultiSelectItem{
		{ID: "bug", Label: "Bug"},
		{ID: "chore", Label: "Chore"},
	}, []string{"chore"}, func([]string) {})

	before := multiSelectRows(app)
	app.multiSelectModal.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if after := multiSelectRows(app); after != before {
		t.Errorf("moving rewrote the rows: %q became %q", before, after)
	}

	app.multiSelectModal.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	first, _ := app.multiSelectModal.list.GetItemText(0)
	second, _ := app.multiSelectModal.list.GetItemText(1)
	if markOf(first) != '◻' {
		t.Errorf("first row = %q, want it untouched by the second row's toggle", first)
	}
	if markOf(second) != '◻' {
		t.Errorf("second row = %q, want it unticked", second)
	}
}

func multiSelectRows(app *App) string {
	rows := ""
	for i := 0; i < app.multiSelectModal.list.GetItemCount(); i++ {
		text, _ := app.multiSelectModal.list.GetItemText(i)
		rows += text + "|"
	}
	return rows
}

// TestAgentOutputFitsTheScreen is the one that did not fit at all: 110x32 is
// larger than a 100x30 terminal before the modal draws anything.
func TestAgentOutputFitsTheScreen(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	app.agentOutputModal.Show("Summarize", func() {})
	t.Cleanup(app.agentOutputModal.Hide)
	app.app.ForceDraw()

	x, y, width, height := app.agentOutputModal.modalContent.GetRect()
	if x < 0 || x+width > 100 {
		t.Errorf("panel spans columns %d..%d on a 100-column screen", x, x+width)
	}
	if y < 0 || y+height > 30 {
		t.Errorf("panel spans rows %d..%d on a 30-row screen", y, y+height)
	}
}

// TestAgentOutputLightsTheViewTabLandsOn covers the focus cue: both views wore
// the accent, so nothing said which one the scroll keys reached.
func TestAgentOutputLightsTheViewTabLandsOn(t *testing.T) {
	app := appOnScreen(t, 120, 40)
	modal := app.agentOutputModal
	modal.Show("Summarize", func() {})
	t.Cleanup(modal.Hide)

	if got := modal.streamView.GetBorderColor(); got != app.theme.BorderFocus {
		t.Errorf("stream border = %v, want the focus color on open", got)
	}
	if got := modal.finalView.GetBorderColor(); got != app.theme.Border {
		t.Errorf("final border = %v, want the resting color on open", got)
	}

	modal.HandleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if got := modal.finalView.GetBorderColor(); got != app.theme.BorderFocus {
		t.Errorf("final border = %v, want the focus color after Tab", got)
	}
	if got := modal.streamView.GetBorderColor(); got != app.theme.Border {
		t.Errorf("stream border = %v, want the resting color after Tab", got)
	}
}

// markOf is the toggle box a row drew, past the color tag in front of it.
func markOf(row string) rune {
	for _, r := range row {
		if r == '◼' || r == '◻' {
			return r
		}
	}
	return 0
}
