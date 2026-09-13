package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	modalGutter        = 1
	modalScreenWMargin = 8
	modalScreenHMargin = 4
	modalMinWidth      = 24
)

func (a *App) modalScreen() (width, height int) {
	_, _, w, h := a.pages.GetRect()
	return w, h
}

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

func (a *App) modalColumnRule(top func() int) *tview.Box {
	rule := tview.NewBox()
	rule.SetBackgroundColor(a.theme.ModalBackground())
	rule.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		style := tcell.StyleDefault.
			Background(a.theme.ModalBackground()).
			Foreground(a.theme.BorderFocus)
		from := y
		if t := top(); t >= 0 {
			if t < from {
				from = t
			}
			screen.SetContent(x, from-1, tview.Borders.TopT, nil, style)
		}
		for row := from; row < y+height; row++ {
			screen.SetContent(x, row, tview.Borders.Vertical, nil, style)
		}
		return x, y, width, height
	})
	return rule
}

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

func (a *App) modalPanel(title string) *tview.Flex {
	panel := tview.NewFlex().SetDirection(tview.FlexRow)
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

	lm.contextView = tview.NewTextView()
	lm.contextView.SetDynamicColors(true)
	lm.contextView.SetBackgroundColor(app.theme.ModalBackground())

	lm.hintView = tview.NewTextView()
	lm.hintView.SetText(hint).
		SetTextColor(app.theme.SecondaryText).
		SetBackgroundColor(app.theme.ModalBackground())
	lm.hintView.SetTextAlign(tview.AlignCenter)

	lm.modal = tview.NewFlex()
	lm.modal.SetBackgroundColor(app.theme.Background)

	return lm
}

func (lm *listModal) showPlaceholder(text string) {
	lm.count = 0
	lm.list.Clear()
	lm.list.AddItem(lm.app.themeTags.SecondaryText+text+"[-]", "", 0, nil)
	lm.list.SetSelectedStyle(tcell.StyleDefault.
		Foreground(lm.app.theme.SecondaryText).
		Background(lm.app.theme.ModalBackground()))
}

func (lm *listModal) beginRows(count int) {
	lm.count = count
	lm.list.Clear()
	lm.list.SetSelectedStyle(selectionStyle(lm.app.theme))
}

func (lm *listModal) open(title, contextLine string) {
	lm.contextLine = contextLine
	lm.contextView.SetText(contextLine)
	lm.layout(title)

	lm.app.pages.AddPage(lm.page, lm.modal, true, true)
	lm.app.pages.SendToFront(lm.page)
	lm.app.app.SetFocus(lm.list)
}

func (lm *listModal) layout(title string) {
	app := lm.app

	rows := min(max(lm.count, 1), lm.maxRows)

	hasContext := lm.contextLine != ""
	fixed := 4
	if hasContext {
		fixed += 2
	}

	gap, slots := app.density.ModalSpacerLines, 1
	if !hasContext {
		slots = 2
	}
	least := fixed + 1
	if want := rows + fixed + slots*gap; app.fitModalHeight(want, least) < want {
		gap = 0
	}

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

	centerModal(lm.modal, panel, func() (int, int) {
		return app.modalWidth(lm.widest), app.fitModalHeight(rows+fixed+slots*gap, least)
	})
}

func (lm *listModal) Hide() {
	lm.app.pages.RemovePage(lm.page)
	lm.app.restoreModalFocus()
}

func (lm *listModal) Focus() { lm.app.app.SetFocus(lm.list) }

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

func centerModal(root *tview.Flex, panel tview.Primitive, fit func() (width, height int)) {
	place := func() {
		width, height := fit()
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
	place()

	fittedW, fittedH := -1, -1
	root.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		if width != fittedW || height != fittedH {
			fittedW, fittedH = width, height
			place()
		}
		return x, y, width, height
	})
}
