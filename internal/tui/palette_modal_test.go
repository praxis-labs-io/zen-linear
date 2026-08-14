package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// paletteRowsFor renders the given commands through the palette and returns
// the rows the list drew, so the assertions read what a user sees.
func paletteRowsFor(t *testing.T, commands []Command) (*App, []string) {
	t.Helper()
	app := openPaletteOn(t, 100, 30)
	app.paletteCtrl = NewPaletteController(commands)
	app.updatePaletteList()

	rows := make([]string, app.paletteList.GetItemCount())
	for i := range rows {
		rows[i], _ = app.paletteList.GetItemText(i)
	}
	return app, rows
}

func TestPaletteRowsSpreadTitleAndShortcut(t *testing.T) {
	app, rows := paletteRowsFor(t, []Command{
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
		if got := tview.TaggedStringWidth(tt.row); got != app.paletteRowWidth() {
			t.Errorf("row %q is %d columns, want the full %d", tt.row, got, app.paletteRowWidth())
		}
	}
}

func TestPaletteRowTruncatesATitleThatWouldReachTheShortcut(t *testing.T) {
	app, rows := paletteRowsFor(t, []Command{
		{ID: "long", Title: strings.Repeat("wide ", 20), ShortcutRune: 'x'},
	})

	row := rows[0]
	if got := tview.TaggedStringWidth(row); got != app.paletteRowWidth() {
		t.Fatalf("row is %d columns, want %d", got, app.paletteRowWidth())
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

// openPaletteOn draws the palette on a screen of the given size and hands back
// the app, so a test reads the panel where it actually landed.
func openPaletteOn(t *testing.T, width, height int) *App {
	t.Helper()
	app := newUXTestApp(t)
	screen := tcell.NewSimulationScreen("UTF-8")
	app.app.SetScreen(screen)
	screen.SetSize(width, height)
	app.app.SetRoot(app.pages, true)
	// The panel sizes itself against the screen, and the screen is only known
	// once something has been drawn on it.
	app.app.ForceDraw()
	app.focusedPane = FocusIssues
	app.openPalette()
	app.app.ForceDraw()
	return app
}

// panelRect is where the palette's bordered panel was drawn, worked back from
// the list inside it: the panel is the list plus a gutter and a border each
// side, and opens a border and a query box above it.
func panelRect(t *testing.T, app *App) (x, y, width int) {
	t.Helper()
	listX, listY, listWidth, _ := app.paletteList.GetRect()
	if listWidth == 0 {
		t.Fatal("the palette was never drawn")
	}
	return listX - modalGutter - 1, listY - paletteQueryBoxRows - 1, listWidth + 2*(modalGutter+1)
}

// TestPaletteFitsASmallScreen pins the panel inside the terminal. Laid out at
// its natural size it was drawn from a negative origin, which took the query
// box off the top on a short screen and the first columns of every row off the
// left on a narrow one.
func TestPaletteFitsASmallScreen(t *testing.T) {
	sizes := []struct{ width, height int }{
		{100, 12},
		{44, 24},
		{40, 10},
		{100, 30},
	}
	for _, size := range sizes {
		app := openPaletteOn(t, size.width, size.height)
		x, y, width := panelRect(t, app)
		if x < 0 || x+width > size.width {
			t.Errorf("on a %dx%d screen the panel spans %d..%d", size.width, size.height, x, x+width)
		}
		if y < 0 {
			t.Errorf("on a %dx%d screen the panel starts at row %d", size.width, size.height, y)
		}
		if rows := app.paletteListRows(paletteMaxVisibleRows); rows+app.paletteChromeLines()+2 > size.height {
			t.Errorf("on a %dx%d screen the panel wants %d rows", size.width, size.height, rows+app.paletteChromeLines()+2)
		}
	}
}

// TestClickingAPaletteRowRunsIt covers the whole click path: the centering has
// to give the list a rect the pointer is in, and the palette has to answer the
// click itself rather than let tview move a highlight the controller cannot see.
func TestClickingAPaletteRowRunsIt(t *testing.T) {
	app := openPaletteOn(t, 100, 30)
	ran := ""
	app.paletteCtrl = NewPaletteController([]Command{
		{ID: "zoom", Title: "Zoom details", Group: GroupView, Run: func(*App) { ran = "zoom" }},
		{ID: "settings", Title: "Settings", Group: GroupApp, Run: func(*App) { ran = "settings" }},
	})
	app.updatePaletteList()
	app.app.ForceDraw()

	// Rows are: View, Zoom details, App, Settings.
	x, top, _, _ := app.paletteList.GetInnerRect()
	clickAt(t, app, x+2, top+3)

	if ran != "settings" {
		t.Errorf("clicking the fourth row ran %q, want settings", ran)
	}
	if app.focusedPane == FocusPalette {
		t.Error("running a command left the palette open")
	}
}

func TestClickingAPaletteHeadingRunsNothing(t *testing.T) {
	app := openPaletteOn(t, 100, 30)
	ran := ""
	app.paletteCtrl = NewPaletteController([]Command{
		{ID: "zoom", Title: "Zoom details", Group: GroupView, Run: func(*App) { ran = "zoom" }},
	})
	app.updatePaletteList()
	app.app.ForceDraw()

	x, top, _, _ := app.paletteList.GetInnerRect()
	clickAt(t, app, x+2, top)

	if ran != "" {
		t.Errorf("clicking the heading ran %q", ran)
	}
	if app.focusedPane != FocusPalette {
		t.Error("clicking the heading closed the palette")
	}
	if got := app.paletteCtrl.Cursor(); got != 1 {
		t.Errorf("the cursor moved to %d, want it to hold at 1", got)
	}
	if got := app.app.GetFocus(); got != app.paletteInput {
		t.Errorf("the click took the keyboard off the query box")
	}
}
