package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The Comments tab is one page: the cards, the box a reply is being written in
// under the thread it answers, and the compose card at the end of everything
// said. All of it scrolls together. A box pinned to the foot of the pane is a
// box that covers the conversation it is about, and a reply written down there
// is written away from the thread it belongs to.
//
// The page is text with holes in it. The cards and the boxes' frames are
// rendered into the text view underneath; the text areas are real widgets drawn
// over the rows their frame left empty. That is what keeps one scroll, one
// measure, and one set of card borders across the lot.

// pageSlot is a live widget's place on the page, in the page's own rows and in
// columns from the left of the measure.
type pageSlot struct {
	primitive tview.Primitive
	row       int
	height    int
	column    int
	width     int
}

// commentsPage draws the card stack and the widgets that sit inside it.
type commentsPage struct {
	*tview.Box

	// view holds the page's text and owns the scrolling, so a card's rows and
	// the scroll offset are measured in the same lines.
	view  *tview.TextView
	slots []pageSlot

	// refit re-renders the page when the measure changes, the same way the
	// description tab re-renders on a resize.
	refit func(width int)

	fitted int
}

func newCommentsPage(view *tview.TextView, refit func(int)) *commentsPage {
	page := &commentsPage{Box: tview.NewBox(), view: view, refit: refit, fitted: -1}
	page.SetBackgroundColor(view.GetBackgroundColor())
	return page
}

// setSlots records where the live widgets landed in the page just rendered.
func (p *commentsPage) setSlots(slots []pageSlot) { p.slots = slots }

// Draw paints the text and then the widgets over the holes it left for them.
func (p *commentsPage) Draw(screen tcell.Screen) {
	p.DrawForSubclass(screen, p)
	x, y, width, height := p.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	measure, gutter := readingMeasure(width)
	if measure != p.fitted {
		p.fitted = measure
		p.refit(measure)
	}

	p.view.SetRect(x+gutter, y, measure, height)
	p.view.Draw(screen)

	top, _ := p.view.GetScrollOffset()
	for _, slot := range p.slots {
		start, rows := slot.row-top, slot.height
		if start+rows > height {
			// Cut off at the bottom: a widget draws from its own first row, so
			// the rows that fit are the right ones.
			rows = height - start
		}
		// Not drawn once its first row is above the pane. A widget has no way to
		// start part way down its own content, so a shortened rect would redraw
		// it from the top and the words would jump as the page scrolled. The
		// card's frame keeps scrolling either way; only the writing waits.
		//
		// A rect is cleared rather than left where it was: the mouse is offered
		// to every slot, and one holding its last position takes clicks over
		// whatever is now drawn there.
		if start < 0 || rows <= 0 {
			slot.primitive.SetRect(0, 0, 0, 0)
			continue
		}
		slot.primitive.SetRect(x+gutter+slot.column, y+start, slot.width, rows)
		slot.primitive.Draw(screen)
	}
}

// Focus hands the keyboard to the text view, which is the page's own content.
// A box is focused by the app directly, so this is only the fallback for a
// focus tview delegates on its own.
func (p *commentsPage) Focus(delegate func(tview.Primitive)) { delegate(p.view) }

// HasFocus reports focus on the page or on anything drawn in it.
func (p *commentsPage) HasFocus() bool {
	if p.view.HasFocus() {
		return true
	}
	for _, slot := range p.slots {
		if slot.primitive.HasFocus() {
			return true
		}
	}
	return false
}

// InputHandler sends keys the app did not claim to the text view, where they
// scroll.
func (p *commentsPage) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return p.view.InputHandler()
}

// MouseHandler offers the mouse to the widgets on the page before the text
// under them, so a click lands on the box it looks like it landed on.
func (p *commentsPage) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if !p.InRect(event.Position()) {
			return false, nil
		}
		for _, slot := range p.slots {
			if consumed, capture := slot.primitive.MouseHandler()(action, event, setFocus); consumed {
				return true, capture
			}
		}
		return p.view.MouseHandler()(action, event, setFocus)
	}
}
