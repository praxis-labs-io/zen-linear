package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

const (
	paletteMaxVisibleRows = 12
	paletteMinVisibleRows = 4
	paletteQueryBoxRows   = 3
	paletteMaxWidth       = 60
	paletteRowGap         = 2
	paletteRowIndent      = "  "
)

func (a *App) paletteRowWidth() int {
	return a.modalWidth(paletteMaxWidth) - 2 - (2 * modalGutter)
}

// tview captures the global primitive background at construction and offers no setter for the inner fill.
func newThemedInputField(fill tcell.Color) *tview.InputField {
	previous := tview.Styles.PrimitiveBackgroundColor
	tview.Styles.PrimitiveBackgroundColor = fill
	field := tview.NewInputField()
	tview.Styles.PrimitiveBackgroundColor = previous
	return field
}

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
	a.paletteSearchFrame.Box = tview.NewBox().SetBackgroundColor(panel)
	a.paletteSearchFrame.
		SetBorder(true).
		SetBorderColor(a.theme.BorderFocus).
		SetBackgroundColor(panel)
	a.paletteSearchFrame.AddItem(a.paletteInput, 0, 1, true)
}

func (a *App) paletteChromeLines() int {
	return paletteQueryBoxRows + 2 + a.density.ModalSpacerLines
}

func (a *App) buildPaletteModal() *tview.Flex {
	a.buildPaletteQueryBox()

	a.paletteList = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.Foreground(a.theme.SecondaryText).Background(a.theme.ModalBackground())).
		SetSelectedStyle(selectionStyle(a.theme)).
		SetHighlightFullLine(true)
	a.paletteList.SetBackgroundColor(a.theme.ModalBackground())
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

	return a.layoutPaletteModal(0)
}

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

func (a *App) layoutPaletteModal(matches int) *tview.Flex {
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
		AddItem(a.paletteList, 0, 1, false).
		AddItem(spacer, a.density.ModalSpacerLines, 0, false).
		AddItem(a.modalRule(), 1, 0, false).
		AddItem(hint, 1, 0, false)

	modal := tview.NewFlex()
	modal.SetBackgroundColor(a.theme.Background)
	centerModal(modal, content, func() (int, int) {
		return a.modalWidth(paletteMaxWidth), a.paletteListRows(matches) + a.paletteChromeLines() + 2
	})

	return modal
}

func (a *App) paletteListRows(matches int) int {
	rows := matches
	if rows > paletteMaxVisibleRows {
		rows = paletteMaxVisibleRows
	}
	if rows < paletteMinVisibleRows {
		rows = paletteMinVisibleRows
	}

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
		pad = 1
	}
	return paletteRowIndent + title + strings.Repeat(" ", pad) + a.themeTags.Accent + shortcut + "[-]"
}

func (a *App) updatePaletteList() {
	a.paletteList.Clear()
	rows := a.paletteCtrl.Rows()

	for _, row := range rows {
		a.paletteList.AddItem(a.paletteRow(row), "", 0, nil)
	}
	if cursor := a.paletteCtrl.Cursor(); cursor >= 0 && cursor < len(rows) {
		a.paletteList.SetCurrentItem(cursor)
	}

	a.paletteModal = a.layoutPaletteModal(len(rows))

	a.pages.RemovePage("palette")
	a.pages.AddPage("palette", a.paletteModal, true, false)
	if a.focusedPane == FocusPalette {
		a.pages.ShowPage("palette")
		a.pages.SendToFront("palette")
	}
}
