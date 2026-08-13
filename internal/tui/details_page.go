package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The details pane is one page: the issue's metadata, its description, the
// comment cards, the box a reply is being written in under the thread it
// answers, and the compose card at the end of everything said. All of it
// scrolls together. Reading an issue and reading what people said about it are
// one act, and a box pinned to the foot of the pane is a box that covers the
// conversation it is about.
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

// detailsPage draws the issue and the widgets that sit inside it.
type detailsPage struct {
	*tview.Box

	// view holds the page's text and owns the scrolling, so a card's rows and
	// the scroll offset are measured in the same lines.
	view  *tview.TextView
	slots []pageSlot

	// refit re-renders the page at a new measure. The draw is the only place
	// the live width is known; refit itself skips a width it already laid out
	// at, so this can be called on every frame.
	refit func(width int)
}

func newDetailsPage(view *tview.TextView, refit func(int)) *detailsPage {
	page := &detailsPage{Box: tview.NewBox(), view: view, refit: refit}
	page.SetBackgroundColor(view.GetBackgroundColor())
	return page
}

// setSlots records where the live widgets landed in the page just rendered.
func (p *detailsPage) setSlots(slots []pageSlot) { p.slots = slots }

// Draw paints the text and then the widgets over the holes it left for them.
func (p *detailsPage) Draw(screen tcell.Screen) {
	p.DrawForSubclass(screen, p)
	x, y, width, height := p.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	measure, gutter := readingMeasure(width)
	p.refit(measure)

	p.view.SetRect(x+gutter, y, measure, height)
	p.view.Draw(screen)

	top, _ := p.view.GetScrollOffset()
	shown := false
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
		shown = shown || slot.primitive.HasFocus()
	}

	// A widget shows the terminal's caret where it draws it, and a widget that
	// does not draw leaves the caret wherever it was last put. Scrolled away
	// from, the box left a block sitting over whatever took its place.
	if !shown && p.focusedSlot() != nil {
		screen.HideCursor()
	}
}

// Focus hands the keyboard to the text view, which is the page's own content.
// A box is focused by the app directly, so this is only the fallback for a
// focus tview delegates on its own.
func (p *detailsPage) Focus(delegate func(tview.Primitive)) { delegate(p.view) }

// HasFocus reports focus on the page or on anything drawn in it.
func (p *detailsPage) HasFocus() bool {
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

// InputHandler hands a key to whichever widget on the page holds the keyboard,
// and to the text underneath when none does, where it scrolls.
//
// The walk is the whole point. tview routes a key from the root down the focus
// chain rather than to the focused primitive itself, so a container that
// answers with one child's handler is a container the other children can never
// be typed in: the box takes the focus, the border lights, and every keystroke
// lands in the text view and is dropped.
func (p *detailsPage) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return p.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if focused := p.focusedSlot(); focused != nil {
			if handler := focused.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
			return
		}
		if handler := p.view.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	})
}

// PasteHandler follows the same chain: a paste belongs to the box being written
// in, not to the page under it.
func (p *detailsPage) PasteHandler() func(string, func(tview.Primitive)) {
	return p.WrapPasteHandler(func(text string, setFocus func(tview.Primitive)) {
		if focused := p.focusedSlot(); focused != nil {
			if handler := focused.PasteHandler(); handler != nil {
				handler(text, setFocus)
			}
		}
	})
}

// focusedSlot is the widget on the page holding the keyboard, or nil.
func (p *detailsPage) focusedSlot() tview.Primitive {
	for _, slot := range p.slots {
		if slot.primitive.HasFocus() {
			return slot.primitive
		}
	}
	return nil
}

// MouseHandler offers the mouse to the widgets on the page before the text
// under them, so a click lands on the box it looks like it landed on.
func (p *detailsPage) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
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
