package tui

import (
	"slices"
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
		if !strings.HasPrefix(tt.row, paletteRowIndent+tt.title) {
			t.Errorf("row %q does not start with %q", tt.row, tt.title)
		}
		if tt.key == "" {
			if tt.row != paletteRowIndent+tt.title {
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

// TestEveryCommandFilesUnderAHeading guards the one silent failure in the
// grouping: a command whose group is missing from commandGroupOrder is listed
// after every headed group with no heading of its own.
func TestEveryCommandFilesUnderAHeading(t *testing.T) {
	app := newUXTestApp(t)
	for _, cmd := range app.paletteCtrl.commands {
		if !slices.Contains(commandGroupOrder, cmd.Group) {
			t.Errorf("command %q has group %q, which the palette draws no heading for", cmd.ID, cmd.Group)
		}
	}
}

func TestPaletteGroupsTheDefaultListAndFlattensAQuery(t *testing.T) {
	app := newUXTestApp(t)
	app.paletteCtrl = NewPaletteController([]Command{
		{ID: "zoom", Title: "Zoom details", Group: GroupView},
		{ID: "settings", Title: "Settings", Group: GroupApp},
		{ID: "favorite", Title: "Favorite item", Group: GroupView},
	})

	rows := app.paletteCtrl.Rows()
	want := []string{"View", "Favorite item", "Zoom details", "App", "Settings"}
	if len(rows) != len(want) {
		t.Fatalf("drew %d rows, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		got := string(row.Heading)
		if !row.IsHeader {
			got = row.Command.Title
		}
		if got != want[i] {
			t.Errorf("row %d is %q, want %q", i, got, want[i])
		}
	}
	if !rows[0].IsHeader || rows[1].IsHeader {
		t.Error("the first row should be a heading and the second a command")
	}

	app.paletteCtrl.SetQuery("e")
	for _, row := range app.paletteCtrl.Rows() {
		if row.IsHeader {
			t.Fatal("a query still drew a heading")
		}
	}
}

func TestPaletteCursorStepsOverHeadings(t *testing.T) {
	pc := NewPaletteController([]Command{
		{ID: "zoom", Title: "Zoom details", Group: GroupView},
		{ID: "settings", Title: "Settings", Group: GroupApp},
	})

	// Rows are: View, Zoom details, App, Settings.
	if got := pc.Cursor(); got != 1 {
		t.Fatalf("opened on row %d, want the first command at 1", got)
	}
	pc.MoveCursorDown()
	if got := pc.Cursor(); got != 3 {
		t.Errorf("stepping down landed on row %d, want 3", got)
	}
	if cmd, ok := pc.Selected(); !ok || cmd.ID != "settings" {
		t.Errorf("selected %v/%v, want settings", cmd.ID, ok)
	}
	pc.MoveCursorDown()
	if got := pc.Cursor(); got != 3 {
		t.Errorf("stepping past the end moved to %d, want to hold at 3", got)
	}
	pc.MoveCursorUp()
	if got := pc.Cursor(); got != 1 {
		t.Errorf("stepping up landed on row %d, want 1", got)
	}
	pc.MoveCursorUp()
	if got := pc.Cursor(); got != 1 {
		t.Errorf("stepping past the top moved to %d, want to hold at 1", got)
	}
}
