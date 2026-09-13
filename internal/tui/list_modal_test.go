package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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

func listPanelRect(t *testing.T, list *tview.List) (x, y, width int) {
	t.Helper()
	listX, listY, listWidth, _ := list.GetRect()
	if listWidth == 0 {
		t.Fatal("the modal was never drawn")
	}
	return listX - modalGutter - 1, listY, listWidth + 2*(modalGutter+1)
}

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

func TestListModalsFitASmallScreen(t *testing.T) {
	sizes := []struct{ width, height int }{
		{100, 12},
		{44, 14},
		{40, 10},
		{60, 8},
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

func assertPanelOnScreen(t *testing.T, list *tview.List, hint *tview.TextView, screenW, screenH int) {
	t.Helper()
	x, listY, width := listPanelRect(t, list)
	hintY, _ := rowsOf(hint)

	if x < 0 || x+width > screenW {
		t.Errorf("panel spans columns %d..%d on a %d-column screen", x, x+width, screenW)
	}
	if listY < 1 {
		t.Errorf("the list starts at row %d, leaving no room for the top border", listY)
	}
	if bottom := hintY + 1; bottom >= screenH {
		t.Errorf("the bottom border lands on row %d of a %d-row screen", bottom, screenH)
	}
	if _, rows := rowsOf(list); rows < 1 {
		t.Errorf("the list drew %d rows, want at least one option visible", rows)
	}
}

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

func TestPickerCapsItsHeight(t *testing.T) {
	app := appOnScreen(t, 100, 40)
	app.pickerModal.Show("Set Status", pickerItems(30), func(PickerItem) {})
	app.app.ForceDraw()
	if _, rows := rowsOf(app.pickerModal.list); rows != pickerMaxVisibleRows {
		t.Errorf("a thirty-option picker drew %d rows, want the %d cap", rows, pickerMaxVisibleRows)
	}
}

func TestPickerNamesItselfOnTheBorderOnly(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	app.pickerModal.Show("Set Priority", pickerItems(3), func(PickerItem) {})
	for i := 0; i < app.pickerModal.list.GetItemCount(); i++ {
		if text, _ := app.pickerModal.list.GetItemText(i); text == "Set Priority" {
			t.Fatalf("row %d repeats the modal title", i)
		}
	}
}

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

func TestAgentOutputKeepsBothViewsReadable(t *testing.T) {
	for _, size := range []struct{ width, height int }{{100, agentOutputLeastHeight}, {60, 20}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			app := appOnScreen(t, size.width, size.height)
			app.agentOutputModal.Show("Claude Output", func() {})
			t.Cleanup(app.agentOutputModal.Hide)
			app.app.ForceDraw()

			if _, rows := rowsOf(app.agentOutputModal.streamView); rows-2 < 1 {
				t.Errorf("the stream view has %d rows inside its border", rows-2)
			}
			if _, rows := rowsOf(app.agentOutputModal.finalView); rows-2 < 1 {
				t.Errorf("the final view has %d rows inside its border", rows-2)
			}
		})
	}
}

func TestAgentOutputTitleIsPaddedOnce(t *testing.T) {
	app := appOnScreen(t, 120, 40)
	app.agentOutputModal.Show("  Claude Output  ", func() {})
	t.Cleanup(app.agentOutputModal.Hide)

	if got := app.agentOutputModal.modalContent.GetTitle(); got != " Claude Output " {
		t.Errorf("panel title = %q, want a single space each side", got)
	}
}

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

func markOf(row string) rune {
	for _, r := range row {
		if r == '◼' || r == '◻' {
			return r
		}
	}
	return 0
}

func TestClickingAPickerRowThatOpensAnother(t *testing.T) {
	app := appOnScreen(t, 100, 30)
	app.pickerModal.Show("Filter Issues", []PickerItem{
		{ID: "a", Label: "Assignee"}, {ID: "b", Label: "Status"}, {ID: "c", Label: "Labels"},
	}, func(PickerItem) {
		app.pickerModal.Show("Status", []PickerItem{
			{ID: "todo", Label: "Todo"}, {ID: "done", Label: "Done"},
		}, func(PickerItem) {})
	})
	app.app.ForceDraw()

	x, y, _, _ := app.pickerModal.list.GetInnerRect()
	event := tcell.NewEventMouse(x+1, y+2, tcell.Button1, tcell.ModNone)
	handler := app.pickerModal.list.MouseHandler()
	handler(tview.MouseLeftClick, event, func(p tview.Primitive) { app.app.SetFocus(p) })

	if got := len(app.pickerModal.items); got != 2 {
		t.Fatalf("the click left %d options, want the chained picker's 2", got)
	}
	if got := app.pickerModal.list.GetCurrentItem(); got < 0 || got >= 2 {
		t.Fatalf("the chained picker's cursor is %d, off a 2-option list", got)
	}
}
