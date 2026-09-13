package tui

import (
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/terminfo"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

const (
	defaultCellWidth  = 8
	defaultCellHeight = 16
)

// Both halves: the same upload twice in a description shares its bytes and not its placement.
type placementKey struct {
	image     uint32
	placement uint32
}

func keyOf(image screenImage) placementKey {
	return placementKey{image: image.id, placement: image.placement}
}

type graphicsState struct {
	protocol   graphicsProtocol
	sent       map[uint32]bool
	placed     map[placementKey]screenImage
	cellWidth  int
	cellHeight int
	screen     tcell.Screen
}

func newGraphicsState() *graphicsState {
	return &graphicsState{
		protocol:   kittyGraphics{},
		sent:       map[uint32]bool{},
		placed:     map[placementKey]screenImage{},
		cellWidth:  defaultCellWidth,
		cellHeight: defaultCellHeight,
	}
}

func (a *App) beginImageFrame() {
	a.pendingImages = nil
}

func (a *App) imagesWanted() []screenImage {
	if a.activeModal() != nil || a.paletteOpen() {
		return nil
	}
	return a.pendingImages
}

// Tears the images down before Stop, which closes the tty.
func (a *App) quit() {
	a.clearImages()
	a.app.Stop()
}

func (a *App) recordImages(images []screenImage) {
	a.pendingImages = images
}

// Runs from SetAfterDrawFunc, the one window after the widgets draw and before screen.Show; writes go to screen.Tty to stay ordered with tcell.
func (a *App) drawImages(screen tcell.Screen) {
	tty, ok := screen.Tty()
	if !ok {
		return
	}
	state := a.graphics
	if state == nil {
		return
	}
	state.screen = screen

	if size, err := tty.WindowSize(); err == nil {
		if width, height := size.CellDimensions(); width > 0 && height > 0 {
			state.cellWidth, state.cellHeight = width, height
		}
	}

	pending := a.imagesWanted()

	wanted := map[placementKey]screenImage{}
	for _, image := range pending {
		wanted[keyOf(image)] = image
	}

	for key, was := range state.placed {
		if now, still := wanted[key]; still && now == was {
			continue
		}
		if err := state.protocol.Delete(tty, key.image, key.placement); err != nil {
			logger.Debug("tui.graphics: delete image id=%d placement=%d error=%v", key.image, key.placement, err)
		}
		delete(state.placed, key)
		screen.LockRegion(was.x, was.y, was.cols, was.rows, false)
	}

	info, err := terminfo.LookupTerminfo(os.Getenv("TERM"))
	if err != nil {
		return
	}

	for _, image := range pending {
		if _, already := state.placed[keyOf(image)]; already {
			screen.LockRegion(image.x, image.y, image.cols, image.rows, true)
			continue
		}
		if !state.sent[image.id] {
			data, err := os.ReadFile(image.path)
			if err != nil {
				logger.Debug("tui.graphics: read image path=%s error=%v", image.path, err)
				continue
			}
			if err := state.protocol.Transmit(tty, image.id, data); err != nil {
				logger.Debug("tui.graphics: transmit image id=%d error=%v", image.id, err)
				continue
			}
			state.sent[image.id] = true
		}

		info.TPuts(tty, info.TGoto(image.x, image.y))
		if err := state.protocol.Place(tty, image.id, image.placement, image.cols, image.rows); err != nil {
			logger.Debug("tui.graphics: place image id=%d error=%v", image.id, err)
			continue
		}
		state.placed[keyOf(image)] = image
		screen.LockRegion(image.x, image.y, image.cols, image.rows, true)
	}
}

func (a *App) clearImages() {
	state := a.graphics
	if state == nil || len(state.placed) == 0 {
		return
	}
	screen := state.screen
	if screen == nil {
		return
	}
	tty, ok := screen.Tty()
	if !ok {
		return
	}
	for key, was := range state.placed {
		if err := state.protocol.Delete(tty, key.image, key.placement); err != nil {
			logger.Debug("tui.graphics: delete image id=%d placement=%d error=%v", key.image, key.placement, err)
		}
		screen.LockRegion(was.x, was.y, was.cols, was.rows, false)
	}
	state.placed = map[placementKey]screenImage{}
	a.pendingImages = nil
}

func (s *graphicsState) imageBox(maxCols, maxRows, width, height int) (cols, rows int) {
	if maxCols <= 0 || maxRows <= 0 || width <= 0 || height <= 0 || s.cellWidth <= 0 || s.cellHeight <= 0 {
		return 0, 0
	}

	cols = maxCols
	rows = ceilDiv(cols*s.cellWidth*height, width*s.cellHeight)
	if rows > maxRows {
		rows = maxRows
		cols = ceilDiv(rows*s.cellHeight*width, height*s.cellWidth)
		if cols > maxCols {
			cols = maxCols
		}
	}
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	return cols, rows
}

func ceilDiv(numerator, denominator int) int {
	if denominator <= 0 {
		return 0
	}
	return (numerator + denominator - 1) / denominator
}

const (
	maxImageRows = 15

	minImageRows = 6
)
