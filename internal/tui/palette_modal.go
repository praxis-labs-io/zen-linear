package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

const (
	// paletteMaxVisibleRows caps how many commands the palette shows at once;
	// longer lists scroll.
	paletteMaxVisibleRows = 12
	// paletteMinVisibleRows keeps the panel from resizing under every
	// keystroke as the match count falls.
	paletteMinVisibleRows = 4
	// paletteQueryBoxRows is the framed field: the input plus its border.
	paletteQueryBoxRows = 3
	// paletteGutter is the column of padding each side of the panel's content,
	// without which the query box's frame sits on the panel's own border.
	paletteGutter   = 1
	paletteMaxWidth = 60
	// paletteMinWidth is the narrowest panel worth drawing. A terminal below it
	// cannot hold the palette whatever it is given, and the row arithmetic
	// stops meaning anything once the shortcut column runs past the title.
	paletteMinWidth = 24
	// The panel gives back this much of a screen too small to hold it at full
	// size, matching what FormModal leaves around its own shell.
	paletteScreenWMargin = 8
	paletteScreenHMargin = 4
	// paletteRowGap is the least space kept between a title and its shortcut,
	// so a long title is truncated rather than run into one.
	paletteRowGap = 2
	// paletteRowIndent sets a command in from the heading above it.
	paletteRowIndent = "  "
)

// paletteScreen is the size the panel lays itself out against.
func (a *App) paletteScreen() (width, height int) {
	_, _, w, h := a.pages.GetRect()
	return w, h
}

// paletteWidth is the panel's width: its natural size, clamped to a screen too
// narrow to hold it. Fixed at the full width, the panel is drawn off the left
// edge and every row loses its first columns.
func (a *App) paletteWidth() int {
	screenW, _ := a.paletteScreen()
	width := screenW - paletteScreenWMargin
	if width > paletteMaxWidth {
		width = paletteMaxWidth
	}
	if width < paletteMinWidth {
		width = paletteMinWidth
	}
	return width
}

// paletteRowWidth is what a command row has to lay out in: the panel less its
// border and both gutters.
func (a *App) paletteRowWidth() int {
	return a.paletteWidth() - 2 - (2 * paletteGutter)
}

// newThemedInputField creates an InputField whose inner text area fills with
// the given background. tview captures the global primitive background at
// construction time and offers no setter for it afterwards, so without this
// the field row renders in the default (possibly transparent) background with
// color chips behind only the typed text.
func newThemedInputField(fill tcell.Color) *tview.InputField {
	previous := tview.Styles.PrimitiveBackgroundColor
	tview.Styles.PrimitiveBackgroundColor = fill
	field := tview.NewInputField()
	tview.Styles.PrimitiveBackgroundColor = previous
	return field
}

// buildPaletteQueryBox creates the palette's query box: the navigation pane's
// framed field, on the modal panel. It always holds the keyboard, so the frame
// takes the focused border outright.
func (a *App) buildPaletteQueryBox() {
	panel := a.theme.ModalBackground()

	a.paletteInput = newThemedInputField(panel)
	a.paletteInput.
		SetLabel("> ").
		SetLabelColor(a.theme.Accent).
		SetFieldWidth(0).
		SetPlaceholder("Search commands").
		SetFieldBackgroundColor(panel).
		SetFieldTextColor(a.theme.Foreground).
		SetPlaceholderTextColor(a.theme.SecondaryText)
	a.paletteInput.SetBackgroundColor(panel)

	a.paletteSearchFrame = tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the
	// fill so the layer beneath cannot bleed through.
	a.paletteSearchFrame.Box = tview.NewBox().SetBackgroundColor(panel)
	a.paletteSearchFrame.
		SetBorder(true).
		SetBorderColor(a.theme.BorderFocus).
		SetBackgroundColor(panel)
	a.paletteSearchFrame.AddItem(a.paletteInput, 0, 1, true)
}

// paletteChromeLines counts the panel's non-list lines inside its border: the
// query box, the footer rule and its hint line, and the density spacer above
// them.
func (a *App) paletteChromeLines() int {
	return paletteQueryBoxRows + 2 + a.density.PaletteSpacerLines
}

// paletteFooterRule is the hint line's top border. It runs out past the gutter
// to the panel's own border on each side, so the rule meets it in a tee rather
// than stopping short of it.
func (a *App) paletteFooterRule() *tview.Box {
	rule := tview.NewBox()
	rule.SetBackgroundColor(a.theme.ModalBackground())
	rule.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		style := tcell.StyleDefault.
			Background(a.theme.ModalBackground()).
			Foreground(a.theme.BorderFocus)
		left, right := x-paletteGutter-1, x+width+paletteGutter
		screen.SetContent(left, y, tview.Borders.LeftT, nil, style)
		for col := left + 1; col < right; col++ {
			screen.SetContent(col, y, tview.Borders.Horizontal, nil, style)
		}
		screen.SetContent(right, y, tview.Borders.RightT, nil, style)
		return x, y, width, height
	})
	return rule
}

// buildPaletteModal creates and configures the command palette modal overlay.
func (a *App) buildPaletteModal() *tview.Flex {
	a.buildPaletteQueryBox()

	a.paletteList = tview.NewList().
		ShowSecondaryText(false).
		// Unselected rows sit back in the muted text color; the selection is
		// what brings a title up to the foreground.
		SetMainTextStyle(tcell.StyleDefault.Foreground(a.theme.SecondaryText).Background(a.theme.ModalBackground())).
		// The same selection the tree and the issue tables paint, so a focused
		// row reads the same wherever it is.
		SetSelectedStyle(selectionStyle(a.theme)).
		SetHighlightFullLine(true)
	a.paletteList.SetBackgroundColor(a.theme.ModalBackground())
	// tview's own click handler takes focus for the list and moves a highlight
	// the controller knows nothing about, and it does both after the callbacks
	// it fires, so nothing a callback sets survives it. Answer the click here
	// and swallow it, and the query box keeps the keyboard and the cursor.
	a.paletteList.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action != tview.MouseLeftClick {
			return action, event
		}
		x, y := event.Position()
		if !a.paletteList.InRect(x, y) {
			return action, event
		}
		a.runPaletteRowAt(x, y)
		return action, nil
	})

	return a.layoutPaletteModal(paletteMinVisibleRows)
}

// runPaletteRowAt runs the command clicked at the given screen cell. A click
// on a heading or past the last row is not something to run, and leaves the
// palette exactly as it was.
func (a *App) runPaletteRowAt(x, y int) {
	left, top, width, height := a.paletteList.GetInnerRect()
	if x < left || x >= left+width || y < top || y >= top+height {
		return
	}
	offset, _ := a.paletteList.GetOffset()
	index := y - top + offset

	rows := a.paletteCtrl.Rows()
	if index < 0 || index >= len(rows) || rows[index].IsHeader {
		return
	}
	a.paletteCtrl.SetCursor(index)
	command := rows[index].Command
	a.closePalette()
	command.Run(a)
}

// layoutPaletteModal rebuilds the panel around a list of the given height and
// centers it. Every keystroke resizes the list, so this runs again on each one;
// the input, its frame, and the list are reused, the wrappers are not.
func (a *App) layoutPaletteModal(listRows int) *tview.Flex {
	panel := a.theme.ModalBackground()

	hint := tview.NewTextView()
	hint.SetText("↑↓ move   ↵ run   esc close").
		SetTextColor(a.theme.SecondaryText).
		SetBackgroundColor(panel)
	hint.SetTextAlign(tview.AlignCenter)

	spacer := tview.NewBox().SetBackgroundColor(panel)

	content := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.paletteSearchFrame, paletteQueryBoxRows, 0, true).
		AddItem(a.paletteList, listRows, 0, false).
		AddItem(spacer, a.density.PaletteSpacerLines, 0, false).
		AddItem(a.paletteFooterRule(), 1, 0, false).
		AddItem(hint, 1, 0, false)
	content.Box = tview.NewBox().SetBackgroundColor(panel)
	content.
		SetBackgroundColor(panel).
		SetBorder(true).
		SetBorderColor(a.theme.BorderFocus).
		// One column of gutter each side, or the query box's frame sits on the
		// panel's own border.
		SetBorderPadding(0, 0, paletteGutter, paletteGutter).
		SetTitle(" Commands ").
		SetTitleColor(a.theme.Foreground)

	// Two Flexes, not three. A third only looks centered because tview hands
	// its spacers a negative width when the panel is wider than the slot they
	// share, which puts the panel where it belongs and its rect somewhere else
	// — and a rect the pointer is not in takes no clicks.
	column := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(content, listRows+a.paletteChromeLines()+2, 0, true).
		AddItem(nil, 0, 1, false)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(column, a.paletteWidth(), 0, true).
		AddItem(nil, 0, 1, false)
	modal.SetBackgroundColor(a.theme.Background)

	return modal
}

// paletteListRows is how many command rows fit: the match count, capped by the
// palette's own limit and by what the screen leaves.
func (a *App) paletteListRows(matches int) int {
	rows := matches
	if rows > paletteMaxVisibleRows {
		rows = paletteMaxVisibleRows
	}
	if rows < paletteMinVisibleRows {
		rows = paletteMinVisibleRows
	}

	// The screen has the last word. A panel taller than the terminal is drawn
	// off the top, taking the query box and the footer with it, so the margin
	// is what gets dropped first and the rows only after that.
	_, screenH := a.paletteScreen()
	if screenH <= 0 {
		return rows
	}
	fits := screenH - a.paletteChromeLines() - 2
	if roomy := fits - paletteScreenHMargin; roomy >= paletteMinVisibleRows {
		fits = roomy
	}
	if fits < 1 {
		fits = 1
	}
	if rows > fits {
		rows = fits
	}
	return rows
}

// paletteRow draws one line of the palette: a heading, or a command with its
// title indented under one and its shortcut against the right edge. A command
// with no shortcut leaves that end blank.
func (a *App) paletteRow(row PaletteRow) string {
	if row.IsHeader {
		return a.themeTags.Accent + string(row.Heading) + "[-]"
	}

	shortcut := row.Command.ShortcutDisplay
	if shortcut == "" {
		shortcut = FormatShortcut(row.Command.ShortcutRune)
	}

	room := a.paletteRowWidth() - len(paletteRowIndent)
	if shortcut == "" {
		return paletteRowIndent + runewidth.Truncate(row.Command.Title, room, "…")
	}
	room -= runewidth.StringWidth(shortcut) + paletteRowGap
	title := runewidth.Truncate(row.Command.Title, room, "…")
	pad := room - runewidth.StringWidth(title) + paletteRowGap
	if pad < 1 {
		// Truncate keeps its ellipsis even when the room is gone, so a panel at
		// the minimum width can still leave the title wider than it was given.
		pad = 1
	}
	return paletteRowIndent + title + strings.Repeat(" ", pad) + a.themeTags.Accent + shortcut + "[-]"
}

// updatePaletteList updates the palette list with filtered commands.
func (a *App) updatePaletteList() {
	a.paletteList.Clear()
	rows := a.paletteCtrl.Rows()

	for _, row := range rows {
		a.paletteList.AddItem(a.paletteRow(row), "", 0, nil)
	}
	if cursor := a.paletteCtrl.Cursor(); cursor >= 0 && cursor < len(rows) {
		a.paletteList.SetCurrentItem(cursor)
	}

	a.paletteModal = a.layoutPaletteModal(a.paletteListRows(len(rows)))

	// Replace the modal in pages
	a.pages.RemovePage("palette")
	a.pages.AddPage("palette", a.paletteModal, true, false)
	if a.focusedPane == FocusPalette {
		a.pages.ShowPage("palette")
		a.pages.SendToFront("palette")
	}
}
