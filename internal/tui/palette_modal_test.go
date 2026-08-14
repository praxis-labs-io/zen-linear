package tui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

// paletteRowsFor renders the given commands through the palette and returns
// the rows the list drew, so the assertions read what a user sees.
func paletteRowsFor(t *testing.T, commands []Command) []string {
	t.Helper()
	app := newUXTestApp(t)
	app.paletteCtrl = NewPaletteController(commands)
	app.updatePaletteList()

	rows := make([]string, app.paletteList.GetItemCount())
	for i := range rows {
		rows[i], _ = app.paletteList.GetItemText(i)
	}
	return rows
}

func TestPaletteRowsSpreadTitleAndShortcut(t *testing.T) {
	rows := paletteRowsFor(t, []Command{
		{ID: "refresh", Title: "Refresh issues", ShortcutRune: 'r'},
		{ID: "search", Title: "Search issues", ShortcutDisplay: "/"},
		{ID: "sort", Title: "Sort issues by…"},
	})
	if len(rows) != 3 {
		t.Fatalf("drew %d rows, want 3", len(rows))
	}

	tests := []struct {
		row   string
		title string
		key   string
	}{
		{rows[0], "Refresh issues", "r"},
		{rows[1], "Search issues", "/"},
		{rows[2], "Sort issues by…", ""},
	}
	for _, tt := range tests {
		if !strings.HasPrefix(tt.row, tt.title) {
			t.Errorf("row %q does not start with %q", tt.row, tt.title)
		}
		if tt.key == "" {
			if tt.row != tt.title {
				t.Errorf("a command with no shortcut drew %q, want the bare title", tt.row)
			}
			continue
		}
		if !strings.HasSuffix(tt.row, tt.key+"[-]") {
			t.Errorf("row %q does not end in its shortcut %q", tt.row, tt.key)
		}
		if got := tview.TaggedStringWidth(tt.row); got != paletteRowWidth {
			t.Errorf("row %q is %d columns, want the full %d", tt.row, got, paletteRowWidth)
		}
	}
}

func TestPaletteRowTruncatesATitleThatWouldReachTheShortcut(t *testing.T) {
	rows := paletteRowsFor(t, []Command{
		{ID: "long", Title: strings.Repeat("wide ", 20), ShortcutRune: 'x'},
	})

	row := rows[0]
	if got := tview.TaggedStringWidth(row); got != paletteRowWidth {
		t.Fatalf("row is %d columns, want %d", got, paletteRowWidth)
	}
	if !strings.Contains(row, "…") {
		t.Errorf("row %q was not truncated", row)
	}
	if !strings.HasSuffix(row, "x[-]") {
		t.Errorf("row %q lost its shortcut", row)
	}
}
