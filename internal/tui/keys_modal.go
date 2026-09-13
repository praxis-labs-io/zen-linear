package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

const (
	keysKeyGutter  = 12
	keysChromeRows = 4
	keysMinWidth   = 34
)

type keySection struct {
	context keyContext
	title   string
	// Resolved at open time, since a settings save rebinds keys mid-session.
	rows func(*App) []hint
}

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
			{"j / k", "move"},
			{"⏎", "open"},
			{"h / l", "collapse, issues pane"},
			{"g / G", "first / last"},
			{"Tab", "search box"},
			{keyPairLabel(a.actionKey("favorite_move_up", 'K'), a.actionKey("favorite_move_down", 'J')), "move a favorite"},
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
			{"g / G", "first / last"},
			{"space", "collapse a group"},
			{keyPairLabel(a.actionKey("columns_left", 'H'), a.actionKey("columns_right", 'L')), "scroll the columns"},
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
			{"(empty)", "clears a due date or estimate"},
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

// The chooser is default-deny and never reaches the shortcut dispatch, so it asks for this rune directly. 0 where a binding took it.
func (a *App) keysReferenceKey() rune {
	key, _ := a.commandShortcutRune("show_keys")
	return key
}

type KeysModal struct {
	app   *App
	modal *tview.Flex
	panel *tview.Flex
	view  *tview.TextView
	hint  *tview.TextView
}

func NewKeysModal(app *App) *KeysModal {
	km := &KeysModal{app: app}

	km.view = tview.NewTextView()
	km.view.SetDynamicColors(true).
		SetWrap(false).
		SetBackgroundColor(app.theme.ModalBackground())

	km.hint = tview.NewTextView()
	km.hint.SetText("esc close").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(app.theme.SecondaryText).
		SetBackgroundColor(app.theme.ModalBackground())

	km.modal = tview.NewFlex()
	km.modal.SetBackgroundColor(app.theme.Background)

	return km
}

func (km *KeysModal) Show() {
	section, ok := keySectionFor(km.app.keyContext())
	if !ok {
		return
	}
	lines := km.app.keyLines(section)
	km.view.SetText(strings.Join(lines, "\n"))
	km.view.ScrollToBeginning()

	km.panel = km.app.modalPanel("Keys: " + section.title)
	km.panel.AddItem(km.view, 0, 1, true)
	km.panel.AddItem(km.app.modalRule(), 1, 0, false)
	km.panel.AddItem(km.hint, 1, 0, false)
	centerModal(km.modal, km.panel, func() (int, int) {
		return km.app.modalWidth(keysPanelWidth(lines)),
			km.app.fitModalHeight(len(lines)+keysChromeRows, keysChromeRows+1)
	})

	km.app.pages.AddPage("keys", km.modal, true, true)
	km.app.pages.SendToFront("keys")
	km.app.app.SetFocus(km.view)
}

func keySectionFor(context keyContext) (keySection, bool) {
	for _, section := range keySections {
		if section.context == context {
			return section, true
		}
	}
	return keySection{}, false
}

func (a *App) keyLines(section keySection) []string {
	lines := a.keyRows(section.rows(a))
	if section.context == keyContextGlobal {
		return lines
	}
	global, ok := keySectionFor(keyContextGlobal)
	if !ok {
		return lines
	}
	lines = append(lines, "", a.themeTags.SecondaryText+global.title+"[-]")
	return append(lines, a.keyRows(global.rows(a))...)
}

func keysPanelWidth(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, tview.TaggedStringWidth(line))
	}
	return max(widest+2+2*modalGutter, keysMinWidth)
}

func (a *App) keyRows(rows []hint) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.key == "" {
			continue
		}
		pad := max(1, keysKeyGutter-runewidth.StringWidth(row.key))
		lines = append(lines, fmt.Sprintf("  %s%s[-]%s%s",
			a.themeTags.Accent, tview.Escape(row.key), strings.Repeat(" ", pad), row.verb))
	}
	return lines
}

func (km *KeysModal) Hide() {
	km.app.pages.RemovePage("keys")
	km.app.restoreModalFocus()
}

func (km *KeysModal) Focus() { km.app.app.SetFocus(km.view) }

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
		default:
			if key := km.app.keysReferenceKey(); key != 0 && event.Rune() == key {
				km.Hide()
			}
		}
	default:
		return event
	}
	return nil
}

func (km *KeysModal) scroll(delta int) {
	row, column := km.view.GetScrollOffset()
	km.view.ScrollTo(max(0, row+delta), column)
}
