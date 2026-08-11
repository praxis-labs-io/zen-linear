package tui

import (
	"strings"
)

// PaletteController manages the command palette filtering and selection logic.
type PaletteController struct {
	commands []Command
	query    string
	cursor   int
	filtered []Command
	// scope is the pane the palette was opened from. Commands outside it are
	// left out of the list rather than ranked down.
	scope CommandScope
}

// SetScope records the scope the palette was opened in and re-filters.
func (p *PaletteController) SetScope(scope CommandScope) {
	p.scope = scope
	p.filterCommands()
}

// inScope drops commands that do not apply where the palette was opened.
func (p *PaletteController) inScope(commands []Command) []Command {
	kept := make([]Command, 0, len(commands))
	for _, cmd := range commands {
		if cmd.appliesIn(p.scope) {
			kept = append(kept, cmd)
		}
	}
	return kept
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
	p.filterCommands()
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
	switch {
	case pos < 0:
		p.cursor = 0
	case pos >= len(p.filtered):
		p.cursor = len(p.filtered) - 1
		if p.cursor < 0 {
			p.cursor = 0
		}
	default:
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
	p.filtered = p.inScope(p.commands)
}

// filterCommands filters commands based on the query.
func (p *PaletteController) filterCommands() {
	if p.query == "" {
		p.filtered = p.inScope(p.commands)
		return
	}

	tokens := strings.Fields(strings.ToLower(p.query))
	filtered := make([]Command, 0)

	for _, cmd := range p.commands {
		searchable := strings.ToLower(cmd.Title + " " + strings.Join(cmd.Keywords, " "))
		matched := true
		for _, token := range tokens {
			if !strings.Contains(searchable, token) {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, cmd)
		}
	}

	p.filtered = p.inScope(filtered)
}
