package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type pageSlot struct {
	primitive tview.Primitive
	row       int
	height    int
	column    int
	width     int
}

type pageImage struct {
	id        uint32
	placement uint32
	path      string
	row       int
	rows      int
	column    int
	cols      int
}

type screenImage struct {
	id        uint32
	placement uint32
	path      string
	x, y      int
	cols      int
	rows      int
}

type detailsPage struct {
	*tview.Box

	view   *tview.TextView
	slots  []pageSlot
	images []pageImage

	refit func(width, height int)

	// Pictures go to the tty as escape sequences, which cannot happen inside tcell's own draw.
	place func([]screenImage)
}

func newDetailsPage(view *tview.TextView, refit func(int, int), place func([]screenImage)) *detailsPage {
	page := &detailsPage{Box: tview.NewBox(), view: view, refit: refit, place: place}
	page.SetBackgroundColor(view.GetBackgroundColor())
	return page
}

func (p *detailsPage) setSlots(slots []pageSlot) { p.slots = slots }

func (p *detailsPage) setImages(images []pageImage) { p.images = images }

func (p *detailsPage) Draw(screen tcell.Screen) {
	p.DrawForSubclass(screen, p)
	x, y, width, height := p.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	measure, gutter := readingMeasure(width)
	p.refit(measure, height)

	p.view.SetRect(x+gutter, y, measure, height)
	p.view.Draw(screen)

	top, _ := p.view.GetScrollOffset()
	shown := false
	for _, slot := range p.slots {
		start, rows := slot.row-top, slot.height
		if start+rows > height {
			rows = height - start
		}
		if start < 0 || rows <= 0 {
			slot.primitive.SetRect(0, 0, 0, 0)
			continue
		}
		slot.primitive.SetRect(x+gutter+slot.column, y+start, slot.width, rows)
		if area, ok := slot.primitive.(*tview.TextArea); ok && rows == slot.height {
			area.SetOffset(0, 0)
		}
		slot.primitive.Draw(screen)
		shown = shown || slot.primitive.HasFocus()
	}

	if !shown && p.focusedSlot() != nil {
		screen.HideCursor()
	}

	p.place(p.visibleImages(x+gutter, y, height, top))
}

// A picture that does not fit whole is dropped, since the terminal scales it into whatever box it is given.
func (p *detailsPage) visibleImages(x, y, height, top int) []screenImage {
	if len(p.images) == 0 {
		return nil
	}
	visible := make([]screenImage, 0, len(p.images))
	for _, image := range p.images {
		start := image.row - top
		if start < 0 || start+image.rows > height || image.cols <= 0 || image.rows <= 0 {
			continue
		}
		visible = append(visible, screenImage{
			id:        image.id,
			placement: image.placement,
			path:      image.path,
			x:         x + image.column,
			y:         y + start,
			cols:      image.cols,
			rows:      image.rows,
		})
	}
	return visible
}

func (p *detailsPage) Focus(delegate func(tview.Primitive)) { delegate(p.view) }

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

// tview routes a key down the focus chain rather than to the focused primitive, so the page forwards to the widget holding focus.
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

func (p *detailsPage) PasteHandler() func(string, func(tview.Primitive)) {
	return p.WrapPasteHandler(func(text string, setFocus func(tview.Primitive)) {
		if focused := p.focusedSlot(); focused != nil {
			if handler := focused.PasteHandler(); handler != nil {
				handler(text, setFocus)
			}
		}
	})
}

func (p *detailsPage) focusedSlot() tview.Primitive {
	for _, slot := range p.slots {
		if slot.primitive.HasFocus() {
			return slot.primitive
		}
	}
	return nil
}

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
