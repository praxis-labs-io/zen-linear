package tui

import (
	"slices"
	"sort"
	"strings"
)

// PaletteRow is one line of the palette: a group heading, or a command. The
// cursor never rests on a heading, so Selected always answers with a command.
type PaletteRow struct {
	Heading  CommandGroup
	Command  Command
	IsHeader bool
}

// PaletteController manages the command palette filtering and selection logic.
type PaletteController struct {
	commands []Command
	query    string
	cursor   int
	filtered []Command
	rows     []PaletteRow
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
	pc := &PaletteController{commands: commands}
	pc.filterCommands()
	return pc
}

// SetCommands swaps in a rebuilt registry, which is how a settings save moves
// shortcuts without a restart. The query and cursor reset with it: the rows
// they pointed at came from the old list.
func (p *PaletteController) SetCommands(commands []Command) {
	p.commands = commands
	p.query = ""
	p.cursor = 0
	p.filterCommands()
}

// SetQuery sets the search query and filters commands.
func (p *PaletteController) SetQuery(q string) {
	p.query = q
	p.filterCommands()
}

// Query returns the current query.
func (p *PaletteController) Query() string {
	return p.query
}

// Filtered returns the filtered list of commands, in the order they are drawn.
func (p *PaletteController) Filtered() []Command {
	return p.filtered
}

// Rows returns the lines the palette draws: the commands, and the headings
// above them while no query narrows the list.
func (p *PaletteController) Rows() []PaletteRow {
	return p.rows
}

// Selected returns the currently selected command and whether one is selected.
func (p *PaletteController) Selected() (Command, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) || p.rows[p.cursor].IsHeader {
		return Command{}, false
	}
	return p.rows[p.cursor].Command, true
}

// Cursor returns the current cursor position.
func (p *PaletteController) Cursor() int {
	return p.cursor
}

// SetCursor sets the cursor position, clamping to valid range and stepping off
// a heading onto the command below it.
func (p *PaletteController) SetCursor(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos >= len(p.rows) {
		pos = len(p.rows) - 1
	}
	if pos < 0 {
		p.cursor = 0
		return
	}
	p.cursor = pos
	if p.rows[pos].IsHeader {
		p.step(1)
	}
}

// MoveCursorUp moves the cursor up by one command.
func (p *PaletteController) MoveCursorUp() { p.step(-1) }

// MoveCursorDown moves the cursor down by one command.
func (p *PaletteController) MoveCursorDown() { p.step(1) }

// step walks the cursor to the next command in the given direction, passing
// over headings. It stays put when there is none, so the ends of the list hold.
func (p *PaletteController) step(delta int) {
	for i := p.cursor + delta; i >= 0 && i < len(p.rows); i += delta {
		if !p.rows[i].IsHeader {
			p.cursor = i
			return
		}
	}
}

// Reset resets the query and cursor to initial state.
func (p *PaletteController) Reset() {
	p.query = ""
	p.filterCommands()
}

// filterCommands rebuilds the rows for the current query and scope, and puts
// the cursor on the first command of the new list.
func (p *PaletteController) filterCommands() {
	matched := p.inScope(p.matching())
	if p.query == "" {
		p.rows = groupedPaletteRows(matched)
	} else {
		p.rows = flatPaletteRows(matched)
	}

	p.filtered = make([]Command, 0, len(matched))
	for _, row := range p.rows {
		if !row.IsHeader {
			p.filtered = append(p.filtered, row.Command)
		}
	}

	p.cursor = 0
	if len(p.rows) > 0 && p.rows[0].IsHeader {
		p.step(1)
	}
}

// matching returns the commands the query selects, in registry order. Every
// whitespace-separated token has to appear somewhere in the title or keywords.
func (p *PaletteController) matching() []Command {
	if p.query == "" {
		return p.commands
	}

	tokens := strings.Fields(strings.ToLower(p.query))
	matched := make([]Command, 0)
	for _, cmd := range p.commands {
		searchable := strings.ToLower(cmd.Title + " " + strings.Join(cmd.Keywords, " "))
		hit := true
		for _, token := range tokens {
			if !strings.Contains(searchable, token) {
				hit = false
				break
			}
		}
		if hit {
			matched = append(matched, cmd)
		}
	}
	return matched
}

// flatPaletteRows lists commands in registry order under no heading.
func flatPaletteRows(commands []Command) []PaletteRow {
	rows := make([]PaletteRow, 0, len(commands))
	for _, cmd := range commands {
		rows = append(rows, PaletteRow{Command: cmd})
	}
	return rows
}

// groupedPaletteRows stacks the commands under their headings, alphabetically
// within each. Commands whose group is not in commandGroupOrder keep registry
// order and follow the headed groups with no heading of their own.
func groupedPaletteRows(commands []Command) []PaletteRow {
	buckets := make(map[CommandGroup][]Command, len(commandGroupOrder))
	loose := make([]Command, 0)
	for _, cmd := range commands {
		if !slices.Contains(commandGroupOrder, cmd.Group) {
			loose = append(loose, cmd)
			continue
		}
		buckets[cmd.Group] = append(buckets[cmd.Group], cmd)
	}

	rows := make([]PaletteRow, 0, len(commands)+len(commandGroupOrder))
	for _, group := range commandGroupOrder {
		bucket := buckets[group]
		if len(bucket) == 0 {
			continue
		}
		sort.Slice(bucket, func(i, j int) bool { return bucket[i].Title < bucket[j].Title })
		rows = append(rows, PaletteRow{Heading: group, IsHeader: true})
		for _, cmd := range bucket {
			rows = append(rows, PaletteRow{Command: cmd})
		}
	}
	return append(rows, flatPaletteRows(loose)...)
}
