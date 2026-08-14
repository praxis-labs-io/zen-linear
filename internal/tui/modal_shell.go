package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The shell every overlay panel is built from: sized against the screen rather
// than fixed, bordered in the focus color, and centered in two Flexes. The
// command palette established the shape; these are the parts of it that are not
// about commands.
const (
	// modalGutter is the column of padding each side of a panel's content,
	// without which a framed child sits on the panel's own border.
	modalGutter = 1
	// A panel gives back this much of a screen too small to hold it at full
	// size, so it never runs to the terminal's edge.
	modalScreenWMargin = 8
	modalScreenHMargin = 4
	// modalMinWidth is the narrowest panel worth drawing. Below it the row
	// arithmetic stops meaning anything.
	modalMinWidth = 24
)

// modalScreen is the size a panel lays itself out against.
func (a *App) modalScreen() (width, height int) {
	_, _, w, h := a.pages.GetRect()
	return w, h
}

// modalWidth is a panel's width: its natural size, clamped to a screen too
// narrow to hold it. Fixed at the full width, the panel is drawn off the left
// edge and every row loses its first columns.
func (a *App) modalWidth(widest int) int {
	screenW, _ := a.modalScreen()
	width := screenW - modalScreenWMargin
	if width > widest {
		width = widest
	}
	if width < modalMinWidth {
		width = modalMinWidth
	}
	return width
}

// fitModalHeight clamps a panel's content height to the screen. A panel taller
// than the terminal is drawn off the top, taking its title and footer with it.
//
// least is the height below which the panel has no body left, chrome plus a
// row of content. The margin is given back before that floor is crossed, and
// the floor before the screen itself: a panel that has to overrun its margin to
// show one option is worth more than a tidy one showing none. Only a terminal
// shorter than the floor clips, and there is nothing else to give.
func (a *App) fitModalHeight(want, least int) int {
	_, screenH := a.modalScreen()
	if screenH <= 0 {
		return want
	}
	if roomy := screenH - modalScreenHMargin; want > roomy {
		want = roomy
	}
	if want < least {
		want = least
	}
	if want > screenH {
		want = screenH
	}
	if want < 1 {
		want = 1
	}
	return want
}

// modalRule is a footer's top border. It runs out past the gutter to the
// panel's own border on each side, so the rule meets it in a tee rather than
// stopping short of it.
func (a *App) modalRule() *tview.Box {
	rule := tview.NewBox()
	rule.SetBackgroundColor(a.theme.ModalBackground())
	rule.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		style := tcell.StyleDefault.
			Background(a.theme.ModalBackground()).
			Foreground(a.theme.BorderFocus)
		left, right := x-modalGutter-1, x+width+modalGutter
		screen.SetContent(left, y, tview.Borders.LeftT, nil, style)
		for col := left + 1; col < right; col++ {
			screen.SetContent(col, y, tview.Borders.Horizontal, nil, style)
		}
		screen.SetContent(right, y, tview.Borders.RightT, nil, style)
		return x, y, width, height
	})
	return rule
}

// modalPanel is an empty bordered panel: the caller stacks its rows. The title
// goes on the border, never in a content row.
func (a *App) modalPanel(title string) *tview.Flex {
	panel := tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the fill
	// so the layer beneath cannot bleed through.
	panel.Box = tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())
	panel.
		SetBackgroundColor(a.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(a.theme.BorderFocus).
		SetBorderPadding(0, 0, modalGutter, modalGutter).
		SetTitle(" " + title + " ").
		SetTitleColor(a.theme.Accent)
	return panel
}

// listModal is the shell a picker-style overlay is drawn in: an optional issue
// context line, a list of options, and one hint line under a rule. The picker
// and the multi-select each embed one and add only their own keys and payload.
type listModal struct {
	app         *App
	page        string
	modal       *tview.Flex
	list        *tview.List
	contextView *tview.TextView
	hintView    *tview.TextView
	contextLine string
	widest      int
	maxRows     int
	count       int
}

// newListModal builds the shell. widest caps the panel's width and maxRows its
// list, so a longer one scrolls rather than filling the terminal.
func newListModal(app *App, page, hint string, widest, maxRows int) *listModal {
	lm := &listModal{app: app, page: page, widest: widest, maxRows: maxRows}

	lm.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.
			Foreground(app.theme.Foreground).
			Background(app.theme.ModalBackground())).
		SetSelectedStyle(selectionStyle(app.theme)).
		SetHighlightFullLine(true)
	lm.list.SetBackgroundColor(app.theme.ModalBackground())

	// Shown only when a Show passes a context line.
	lm.contextView = tview.NewTextView()
	lm.contextView.SetDynamicColors(true)
	lm.contextView.SetBackgroundColor(app.theme.ModalBackground())

	lm.hintView = tview.NewTextView()
	lm.hintView.SetText(hint).
		SetTextColor(app.theme.SecondaryText).
		SetBackgroundColor(app.theme.ModalBackground())
	lm.hintView.SetTextAlign(tview.AlignCenter)

	// The panel is rebuilt per Show, since its title and height come from what
	// it is asked to hold. The wrapper is not: pages hold this pointer.
	lm.modal = tview.NewFlex()
	lm.modal.SetBackgroundColor(app.theme.Background)

	return lm
}

// showPlaceholder puts one dim row in the list and hides the cursor behind it,
// so nothing reads as pickable when there is nothing to pick.
func (lm *listModal) showPlaceholder(text string) {
	lm.count = 0
	lm.list.Clear()
	lm.list.AddItem(lm.app.themeTags.SecondaryText+text+"[-]", "", 0, nil)
	lm.list.SetSelectedStyle(tcell.StyleDefault.
		Foreground(lm.app.theme.SecondaryText).
		Background(lm.app.theme.ModalBackground()))
}

// beginRows clears the list for count options and puts the cursor back, after
// a placeholder may have hidden it.
func (lm *listModal) beginRows(count int) {
	lm.count = count
	lm.list.Clear()
	lm.list.SetSelectedStyle(selectionStyle(lm.app.theme))
}

// open lays the panel out for what the list now holds and raises the page.
func (lm *listModal) open(title, contextLine string) {
	lm.contextLine = contextLine
	lm.contextView.SetText(contextLine)
	lm.layout(title)

	lm.app.pages.AddPage(lm.page, lm.modal, true, true)
	lm.app.pages.SendToFront(lm.page)
	lm.app.app.SetFocus(lm.list)
}

// layout sizes the panel to what it holds and centers it. The list is the
// flexible row, so a screen too short takes rows off it rather than clipping
// the title or the hint. Below that the gaps go, and only then does the panel
// overrun its margin: the list is the one part that has to be there.
func (lm *listModal) layout(title string) {
	app := lm.app

	rows := lm.count
	if rows == 0 {
		rows = 1
	}
	if rows > lm.maxRows {
		rows = lm.maxRows
	}

	hasContext := lm.contextLine != ""
	// The border, the footer's rule and its hint line, and where there is one,
	// the context line plus the gap it brings with it.
	fixed := 4
	if hasContext {
		fixed += 2
	}

	// A blank row under the list always, and one above it where no context line
	// stands there. They are the first thing given up on a short screen, being
	// the only part nobody reads.
	gap, slots := app.density.ModalSpacerLines, 1
	if !hasContext {
		slots = 2
	}
	least := fixed + 1
	if want := rows + fixed + slots*gap; app.fitModalHeight(want, least) < want {
		gap = 0
	}
	height := app.fitModalHeight(rows+fixed+slots*gap, least)

	panel := app.modalPanel(title)
	if hasContext {
		panel.AddItem(lm.contextView, 1, 0, false)
		panel.AddItem(nil, 1, 0, false)
	} else {
		panel.AddItem(nil, gap, 0, false)
	}
	panel.AddItem(lm.list, 0, 1, true)
	panel.AddItem(nil, gap, 0, false)
	panel.AddItem(app.modalRule(), 1, 0, false)
	panel.AddItem(lm.hintView, 1, 0, false)

	centerModal(lm.modal, panel, app.modalWidth(lm.widest), height)
}

// Hide closes the overlay and hands the keys back to whatever it covered.
func (lm *listModal) Hide() {
	lm.app.pages.RemovePage(lm.page)
	lm.app.restoreModalFocus()
}

// Focus returns keyboard focus to the list, for when an overlay closes.
func (lm *listModal) Focus() { lm.app.app.SetFocus(lm.list) }

// move steps the cursor without wrapping, and stands down over a placeholder.
func (lm *listModal) move(delta int) {
	if lm.count == 0 {
		return
	}
	index := lm.list.GetCurrentItem() + delta
	if index < 0 || index >= lm.count {
		return
	}
	lm.list.SetCurrentItem(index)
}

// centerModal fills the given wrapper with the panel centered at that size.
// Two Flexes, not three: a third only looks centered because tview hands its
// spacers a negative width when the panel is wider than the slot they share,
// which puts the panel where it belongs and its rect somewhere else, and a rect
// the pointer is not in takes no clicks. The wrapper is refilled rather than
// replaced so a page already holding it keeps drawing.
func centerModal(root *tview.Flex, panel tview.Primitive, width, height int) {
	column := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(panel, height, 0, true).
		AddItem(nil, 0, 1, false)

	root.Clear()
	root.SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(column, width, 0, true).
		AddItem(nil, 0, 1, false)
}
