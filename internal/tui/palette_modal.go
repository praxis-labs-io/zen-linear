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
	paletteMaxWidth     = 60
	// paletteRowGap is the least space kept between a title and its shortcut,
	// so a long title is truncated rather than run into one.
	paletteRowGap = 2
	// paletteRowIndent sets a command in from the heading above it.
	paletteRowIndent = "  "
)

// paletteRowWidth is what a command row has to lay out in: the panel less its
// border and both gutters.
func (a *App) paletteRowWidth() int {
	return a.modalWidth(paletteMaxWidth) - 2 - (2 * modalGutter)
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
	return paletteQueryBoxRows + 2 + a.density.ModalSpacerLines
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

	content := a.modalPanel("Commands")
	content.
		AddItem(a.paletteSearchFrame, paletteQueryBoxRows, 0, true).
		AddItem(a.paletteList, listRows, 0, false).
		AddItem(spacer, a.density.ModalSpacerLines, 0, false).
		AddItem(a.modalRule(), 1, 0, false).
		AddItem(hint, 1, 0, false)

	modal := tview.NewFlex()
	modal.SetBackgroundColor(a.theme.Background)
	centerModal(modal, content, a.modalWidth(paletteMaxWidth), listRows+a.paletteChromeLines()+2)

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
	_, screenH := a.modalScreen()
	if screenH <= 0 {
		return rows
	}
	fits := screenH - a.paletteChromeLines() - 2
	if roomy := fits - modalScreenHMargin; roomy >= paletteMinVisibleRows {
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
