package tui

import (
	"strings"
)

// PaletteController manages the command palette filtering and selection logic.
type PaletteController struct {
	commands     []Command
	query        string
	cursor       int
	filtered     []Command
	isSearchMode bool
}

// NewPaletteController creates a new palette controller with the given commands.
func NewPaletteController(commands []Command) *PaletteController {
	pc := &PaletteController{
		commands: commands,
		filtered: commands,
	}
	return pc
}

// SetQuery sets the search query and filters commands.
func (p *PaletteController) SetQuery(q string) {
	p.query = q
	if !p.isSearchMode {
		p.filterCommands()
	}
	p.cursor = 0
}

// Query returns the current query.
func (p *PaletteController) Query() string {
	return p.query
}

// Filtered returns the filtered list of commands.
func (p *PaletteController) Filtered() []Command {
	return p.filtered
}

// Selected returns the currently selected command and whether one is selected.
func (p *PaletteController) Selected() (Command, bool) {
	if p.isSearchMode {
		return Command{}, false
	}
	if len(p.filtered) == 0 || p.cursor < 0 || p.cursor >= len(p.filtered) {
		return Command{}, false
	}
	return p.filtered[p.cursor], true
}

// Cursor returns the current cursor position.
func (p *PaletteController) Cursor() int {
	return p.cursor
}

// SetCursor sets the cursor position, clamping to valid range.
func (p *PaletteController) SetCursor(pos int) {
	if pos < 0 {
		p.cursor = 0
	} else if pos >= len(p.filtered) {
		p.cursor = len(p.filtered) - 1
		if p.cursor < 0 {
			p.cursor = 0
		}
	} else {
		p.cursor = pos
	}
}

// MoveCursorUp moves the cursor up by one.
func (p *PaletteController) MoveCursorUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

// MoveCursorDown moves the cursor down by one.
func (p *PaletteController) MoveCursorDown() {
	if p.cursor < len(p.filtered)-1 {
		p.cursor++
	}
}

// Reset resets the query and cursor to initial state.
func (p *PaletteController) Reset() {
	p.query = ""
	p.cursor = 0
	p.filtered = p.commands
	p.isSearchMode = false
}

// SetSearchMode sets whether the palette is in search mode.
// In search mode, the query is used for issue search, not command filtering.
func (p *PaletteController) SetSearchMode(mode bool) {
	p.isSearchMode = mode
	if mode {
		p.filtered = nil
	} else {
		p.filtered = p.commands
	}
}

// IsSearchMode returns whether the palette is in search mode.
func (p *PaletteController) IsSearchMode() bool {
	return p.isSearchMode
}

// filterCommands filters commands based on the query.
func (p *PaletteController) filterCommands() {
	if p.query == "" {
		p.filtered = p.commands
		return
	}

	query := strings.ToLower(p.query)
	filtered := make([]Command, 0)

	for _, cmd := range p.commands {
		matched := false
		if strings.Contains(strings.ToLower(cmd.Title), query) {
			matched = true
		} else {
			for _, keyword := range cmd.Keywords {
				if strings.Contains(strings.ToLower(keyword), query) {
					matched = true
					break
				}
			}
		}
		if matched {
			filtered = append(filtered, cmd)
		}
	}

	p.filtered = filtered
}
