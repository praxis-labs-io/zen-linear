package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

const (
	// keysKeyGutter pads the key out so the verbs line up as a column, the way
	// the details grid pads its labels.
	keysKeyGutter = 12
	// keysModalLeastHeight is the border, the footer's rule and hint, and a row
	// of content under them.
	keysModalLeastHeight = 5
	// keysColumnGap separates two columns of sections.
	keysColumnGap = 4
	// keysMaxColumns keeps the panel a panel. Wider than three columns it is
	// the screen, and the eye has to travel further than the scroll it saved.
	keysMaxColumns = 3
	// keysColumnCap is the widest a column is measured as, so one long verb
	// cannot cost the reader a whole column.
	keysColumnCap = 40
)

// keySection is one context's keys. rows is a function because every key is
// resolved when the modal opens: a settings save rebinds them mid-session.
type keySection struct {
	context keyContext
	title   string
	rows    func(*App) []hint
}

// keySections is the reference, in reading order: what works everywhere, then
// the panes left to right, then what a pane opens on top of itself.
var keySections = []keySection{
	{keyContextGlobal, "Anywhere", func(a *App) []hint {
		return []hint{
			a.actionHint("open_palette", ':', "command palette"),
			a.actionHint("search", '/', "search issues"),
			a.commandHint("show_keys", "these keys"),
			a.actionHint("focus_navigation", '1', "navigation pane"),
			a.actionHint("focus_issues", '2', "issues pane"),
			a.actionHint("focus_details", '3', "details pane"),
			{"h / l", "previous / next pane"},
			a.actionHint("quit", 'q', "quit"),
			{"⌃C", "quit"},
		}
	}},
	{keyContextNavigation, "Navigation tree", func(a *App) []hint {
		return []hint{
			{"j / k", "move, and out of the top"},
			{"⏎", "open"},
			{"h / l", "collapse, issues pane"},
			{"g / G", "first / last"},
			{"Tab", "search box"},
			{keyPairLabel(a.actionKey("favorite_move_up", 'K'), a.actionKey("favorite_move_down", 'J')), "move a favorite"},
			{keyPairLabel(a.actionKey("favorite_unnest", 'H'), a.actionKey("favorite_nest", 'L')), "unnest / nest a favorite"},
			a.commandHint("toggle_navigation_pane", "hide the pane"),
		}
	}},
	{keyContextNavSearch, "Navigation search box", func(*App) []hint {
		return []hint{
			{"⏎", "results"},
			{"↓ / Tab", "tree"},
			{"Esc", "clear, then the tree"},
		}
	}},
	{keyContextIssues, "Issue list", func(a *App) []hint {
		return []hint{
			{"j / k", "move"},
			{"⏎", "preview"},
			{"h / l", "previous / next pane"},
			{"space", "collapse a group"},
			{keyPairLabel(a.actionKey("columns_left", '['), a.actionKey("columns_right", ']')), "columns"},
			{"Esc", "leave search results"},
			a.commandHint("zoom_details", "zoom the details pane"),
		}
	}},
	{keyContextDetails, "Details page", func(a *App) []hint {
		return []hint{
			{"j / k", "scroll"},
			{"g / G", "top / bottom"},
			{"⌃D / ⌃U", "half a page"},
			{keyPairLabel(a.actionKey("comment_prev", '{'), a.actionKey("comment_next", '}')), "step the comments"},
			{"h / ←", "issues pane"},
			{"⏎", "close the pane or zoom"},
			{"Esc", "leave the zoom"},
			a.commandHint("zoom_details", "zoom the pane"),
			a.commandHint("edit_issue", "edit the fields in place"),
		}
	}},
	{keyContextComment, "A picked comment", func(a *App) []hint {
		return []hint{
			a.actionHint("comment_reply", 'r', "reply"),
			a.actionHint("comment_quote", 'Q', "quote"),
			a.actionHint("comment_edit", 'e', "edit, if it is yours"),
			a.actionHint("comment_delete", 'd', "delete, if it is yours"),
			a.actionHint("comment_copy_link", 'y', "copy a link"),
			a.actionHint("comment_open", 'o', "open in Linear"),
			{"Esc", "let go of the card"},
		}
	}},
	{keyContextEditMode, "Edit mode", func(a *App) []hint {
		return []hint{
			{"j / k", "step the fields"},
			{"⏎", "open the field"},
			{"Esc", "leave the mode"},
		}
	}},
	{keyContextChooser, "Field chooser", func(*App) []hint {
		return []hint{
			{"j / k", "step the options"},
			{"⏎", "set it, or apply labels"},
			{"space", "toggle a label"},
			{"Esc", "close, saving nothing"},
		}
	}},
	{keyContextFieldEditor, "Title, due date, estimate", func(*App) []hint {
		return []hint{
			{"⏎", "save"},
			{"Esc", "close, saving nothing"},
			{"(empty)", "clears the field"},
		}
	}},
	{keyContextDescription, "Description box", func(*App) []hint {
		return []hint{
			{"⌃S", "save"},
			{"⌃⏎", "also saves"},
			{"⏎", "a newline"},
			{"Esc", "close, dropping the edit"},
		}
	}},
	{keyContextWriting, "Comment box", func(*App) []hint {
		return []hint{
			{"⌃⏎", "post"},
			{"Tab", "the Post button, and back"},
			{"⏎", "a newline, or post"},
			{"⌃C", "copy the selection"},
			{"Esc", "close the box"},
		}
	}},
	{keyContextPalette, "Command palette", func(*App) []hint {
		return []hint{
			{"↑ / ↓", "move"},
			{"⏎", "run"},
			{"Esc", "close"},
		}
	}},
}

// keysReferenceKey is the rune for the two handlers that are default-deny and
// so never reach the shortcut dispatch. 0 where a binding took it, as actionKey.
func (a *App) keysReferenceKey() rune {
	key, _ := a.commandShortcutRune("show_keys")
	return key
}

// KeysModal is the keys reference: one scrolling page of sections, opened on
// the one the reader is in.
type KeysModal struct {
	app    *App
	modal  *tview.Flex
	panel  *tview.Flex
	view   *tview.TextView
	hint   *tview.TextView
	blocks [][]string
}

// NewKeysModal builds the reference.
func NewKeysModal(app *App) *KeysModal {
	km := &KeysModal{app: app}

	km.view = tview.NewTextView()
	km.view.SetDynamicColors(true).
		SetWrap(false).
		SetBackgroundColor(app.theme.ModalBackground())

	km.hint = tview.NewTextView()
	km.hint.SetText("j/k scroll   esc close").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(app.theme.SecondaryText).
		SetBackgroundColor(app.theme.ModalBackground())

	// Rebuilt per Show, since the height comes from what it holds. The wrapper
	// is not: pages hold this pointer.
	km.modal = tview.NewFlex()
	km.modal.SetBackgroundColor(app.theme.Background)

	return km
}

// Show opens the reference on the context the keyboard is in.
func (km *KeysModal) Show() {
	km.blocks = km.app.keyBlocks(km.app.keyContext())
	km.view.ScrollToBeginning()

	km.panel = km.app.modalPanel("Keys")
	km.panel.AddItem(km.view, 0, 1, true)
	km.panel.AddItem(km.app.modalRule(), 1, 0, false)
	km.panel.AddItem(km.hint, 1, 0, false)
	// The columns are re-laid here rather than above, since how many there are
	// is the screen's to decide and centerModal asks again on every resize.
	centerModal(km.modal, km.panel, func() (int, int) {
		lines, width := km.app.keysPage(km.blocks)
		km.view.SetText(strings.Join(lines, "\n"))
		return width, km.app.fitModalHeight(len(lines)+keysModalLeastHeight, keysModalLeastHeight)
	})

	km.app.pages.AddPage("keys", km.modal, true, true)
	km.app.pages.SendToFront("keys")
	km.app.app.SetFocus(km.view)
}

// keyBlocks is one block of lines per section, the reader's own first and the
// rest in the table's order under it.
func (a *App) keyBlocks(current keyContext) [][]string {
	ordered := make([]keySection, 0, len(keySections))
	for _, section := range keySections {
		if section.context == current {
			ordered = append(ordered, section)
		}
	}
	for _, section := range keySections {
		if section.context != current {
			ordered = append(ordered, section)
		}
	}

	blocks := make([][]string, 0, len(ordered))
	for _, section := range ordered {
		block := []string{a.keyHeading(section, section.context == current)}
		blocks = append(blocks, append(block, a.keyRows(section.rows(a))...))
	}
	return blocks
}

// keysPage lays the blocks into as many columns as the screen affords, and
// reports the panel width those columns need.
func (a *App) keysPage(blocks [][]string) (lines []string, width int) {
	column := keysColumnWidth(blocks)
	screenW, _ := a.modalScreen()
	// The screen less its margin, the panel's two borders and its two gutters.
	room := screenW - modalScreenWMargin - 2 - 2*modalGutter
	count := max(1, min((room+keysColumnGap)/(column+keysColumnGap), keysMaxColumns))

	columns := packKeyBlocks(blocks, count)
	return joinKeyColumns(columns, column), a.modalWidth(count*column + (count-1)*keysColumnGap)
}

// keysColumnWidth is the widest row there is, capped so one long verb cannot
// cost the reader a whole column.
func keysColumnWidth(blocks [][]string) int {
	widest := 0
	for _, block := range blocks {
		for _, line := range block {
			widest = max(widest, tview.TaggedStringWidth(line))
		}
	}
	return min(max(widest, 1), keysColumnCap)
}

// packKeyBlocks fills count columns to about an equal share, starting a new one
// rather than breaking a section across the gap.
func packKeyBlocks(blocks [][]string, count int) [][]string {
	total := 0
	for _, block := range blocks {
		total += len(block) + 1
	}
	target := (total + count - 1) / count

	columns := make([][]string, 0, count)
	current := make([]string, 0, target)
	for _, block := range blocks {
		// The last column takes whatever is left, or a block that no longer
		// fits would be dropped rather than scrolled to.
		full := len(current) > 0 && len(current)+len(block) > target
		if full && len(columns) < count-1 {
			columns = append(columns, current)
			current = make([]string, 0, target)
		}
		if len(current) > 0 {
			current = append(current, "")
		}
		current = append(current, block...)
	}
	return append(columns, current)
}

// joinKeyColumns lays the columns side by side, each padded to width. Trailing
// space is cut: the view draws it, and a selection would carry it.
func joinKeyColumns(columns [][]string, width int) []string {
	tallest := 0
	for _, column := range columns {
		tallest = max(tallest, len(column))
	}

	gap := strings.Repeat(" ", keysColumnGap)
	lines := make([]string, 0, tallest)
	for row := 0; row < tallest; row++ {
		cells := make([]string, 0, len(columns))
		for _, column := range columns {
			cell := ""
			if row < len(column) {
				cell = truncateTagged(column[row], width)
			}
			cells = append(cells, cell+strings.Repeat(" ", max(0, width-tview.TaggedStringWidth(cell))))
		}
		lines = append(lines, strings.TrimRight(strings.Join(cells, gap), " "))
	}
	return lines
}

// keyHeading names a section, marking the one the reader is in.
func (a *App) keyHeading(section keySection, current bool) string {
	if !current {
		return a.themeTags.SecondaryText + section.title + "[-]"
	}
	return fmt.Sprintf("%s%s[-]%s  ← you are here[-]", a.themeTags.Accent, section.title, a.themeTags.SecondaryText)
}

// keyRows renders one section's pairs, dropping any left keyless because a
// binding took the rune. Same rule as hintLine: never state a dead key.
func (a *App) keyRows(rows []hint) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.key == "" {
			continue
		}
		key := tview.Escape(row.key)
		pad := max(1, keysKeyGutter-runewidth.StringWidth(row.key))
		lines = append(lines, fmt.Sprintf("  %s%s[-]%s%s", a.themeTags.Accent, key, strings.Repeat(" ", pad), row.verb))
	}
	return lines
}

// Hide closes the reference and hands the keys back to whatever it covered.
func (km *KeysModal) Hide() {
	km.app.pages.RemovePage("keys")
	km.app.restoreModalFocus()
}

// Focus returns keyboard focus to the page, for when an overlay closes.
func (km *KeysModal) Focus() { km.app.app.SetFocus(km.view) }

// HandleKey scrolls the page and closes it. Default-deny: the reference is not
// a place to run anything from, and the key that opened it closes it again.
func (km *KeysModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		return event
	case tcell.KeyEscape:
		km.Hide()
	case tcell.KeyRune:
		switch event.Rune() {
		case 'j':
			km.scroll(1)
		case 'k':
			km.scroll(-1)
		case 'g':
			km.view.ScrollToBeginning()
		case 'G':
			km.view.ScrollToEnd()
		default:
			if key, ok := km.app.commandShortcutRune("show_keys"); ok && event.Rune() == key {
				km.Hide()
			}
		}
	default:
		// The arrows and the page keys belong to the view under the cursor.
		return event
	}
	return nil
}

// scroll steps the page, since the view answers arrows and not j/k.
func (km *KeysModal) scroll(delta int) {
	row, column := km.view.GetScrollOffset()
	km.view.ScrollTo(max(0, row+delta), column)
}
