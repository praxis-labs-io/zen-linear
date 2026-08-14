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
	paletteGutter = 1
	paletteWidth  = 60
	// paletteRowWidth is what a command row has to lay out in: the panel less
	// its border and both gutters.
	paletteRowWidth = paletteWidth - 2 - (2 * paletteGutter)
	// paletteRowGap is the least space kept between a title and its shortcut,
	// so a long title is truncated rather than run into one.
	paletteRowGap = 2
	// paletteRowIndent sets a command in from the heading above it.
	paletteRowIndent = "  "
)

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

	return a.layoutPaletteModal(paletteMinVisibleRows)
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

	a.paletteModalContent = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.paletteSearchFrame, paletteQueryBoxRows, 0, true).
		AddItem(a.paletteList, listRows, 0, false).
		AddItem(spacer, a.density.PaletteSpacerLines, 0, false).
		AddItem(a.paletteFooterRule(), 1, 0, false).
		AddItem(hint, 1, 0, false)
	a.paletteModalContent.Box = tview.NewBox().SetBackgroundColor(panel)
	a.paletteModalContent.
		SetBackgroundColor(panel).
		SetBorder(true).
		SetBorderColor(a.theme.BorderFocus).
		// One column of gutter each side, or the query box's frame sits on the
		// panel's own border.
		SetBorderPadding(0, 0, paletteGutter, paletteGutter).
		SetTitle(" Commands ").
		SetTitleColor(a.theme.Foreground)

	column := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(a.paletteModalContent, listRows+a.paletteChromeLines()+2, 0, true).
		AddItem(nil, 0, 1, false)

	centered := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(column, paletteWidth, 0, true).
		AddItem(nil, 0, 1, false)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(centered, 0, 1, true).
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
	chrome := a.paletteChromeLines() + 2 // + border
	if _, _, _, screenH := a.pages.GetRect(); screenH > chrome+4 {
		if fits := screenH - chrome - 2; rows > fits {
			rows = fits
		}
	}
	if rows < paletteMinVisibleRows {
		rows = paletteMinVisibleRows
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

	room := paletteRowWidth - len(paletteRowIndent) - runewidth.StringWidth(shortcut) - paletteRowGap
	title := runewidth.Truncate(row.Command.Title, room, "…")
	if shortcut == "" {
		return paletteRowIndent + title
	}
	pad := room - runewidth.StringWidth(title) + paletteRowGap
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
