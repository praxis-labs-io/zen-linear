package tui

import (
	"slices"
	"sort"
)

// PaletteRow is a group heading or a command; the cursor never rests on a heading.
type PaletteRow struct {
	Heading  CommandGroup
	Command  Command
	IsHeader bool
}

type PaletteController struct {
	commands []Command
	query    string
	cursor   int
	filtered []Command
	rows     []PaletteRow
	scope    CommandScope
}

func (p *PaletteController) SetScope(scope CommandScope) {
	p.scope = scope
	p.filterCommands()
}

func (p *PaletteController) inScope(commands []Command) []Command {
	kept := make([]Command, 0, len(commands))
	for _, cmd := range commands {
		if cmd.appliesIn(p.scope) {
			kept = append(kept, cmd)
		}
	}
	return kept
}

func NewPaletteController(commands []Command) *PaletteController {
	pc := &PaletteController{commands: commands}
	pc.filterCommands()
	return pc
}

// SetCommands swaps in a rebuilt registry and resets the query and cursor.
func (p *PaletteController) SetCommands(commands []Command) {
	p.commands = commands
	p.query = ""
	p.cursor = 0
	p.filterCommands()
}

func (p *PaletteController) SetQuery(q string) {
	p.query = q
	p.filterCommands()
}

func (p *PaletteController) Query() string {
	return p.query
}

func (p *PaletteController) Filtered() []Command {
	return p.filtered
}

func (p *PaletteController) Rows() []PaletteRow {
	return p.rows
}

func (p *PaletteController) Selected() (Command, bool) {
	if p.cursor < 0 || p.cursor >= len(p.rows) || p.rows[p.cursor].IsHeader {
		return Command{}, false
	}
	return p.rows[p.cursor].Command, true
}

func (p *PaletteController) Cursor() int {
	return p.cursor
}

// SetCursor clamps pos to the rows and steps off a heading onto the command below it.
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

func (p *PaletteController) MoveCursorUp() { p.step(-1) }

func (p *PaletteController) MoveCursorDown() { p.step(1) }

func (p *PaletteController) step(delta int) {
	for i := p.cursor + delta; i >= 0 && i < len(p.rows); i += delta {
		if !p.rows[i].IsHeader {
			p.cursor = i
			return
		}
	}
}

func (p *PaletteController) Reset() {
	p.query = ""
	p.filterCommands()
}

func (p *PaletteController) filterCommands() {
	matched := rankCommands(p.inScope(p.commands), p.query)
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

func flatPaletteRows(commands []Command) []PaletteRow {
	rows := make([]PaletteRow, 0, len(commands))
	for _, cmd := range commands {
		rows = append(rows, PaletteRow{Command: cmd})
	}
	return rows
}

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
