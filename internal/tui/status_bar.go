package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildStatusBar builds the strip of hints under the panes. The text style
// carries the background as well: tview repaints a TextView's inner rect in its
// text style whenever that differs from the box, which left the padding columns
// as blocks in a second color.
func (a *App) buildStatusBar() {
	a.statusBar = tview.NewTextView()
	a.statusBar.SetDynamicColors(true).
		SetWrap(false).
		SetTextStyle(tcell.StyleDefault.Background(a.theme.Background).Foreground(a.theme.SecondaryText))
	a.statusBar.SetBorder(false).SetBackgroundColor(a.theme.Background)

	a.applyStatusBarPadding()
}

func (a *App) applyStatusBarPadding() {
	padding := a.density.StatusBarPadding
	a.statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
}

// keyPairLabel renders two action keys as one hint, dropping either one that
// resolved to nothing because a binding took its rune.
func keyPairLabel(first, second rune) string {
	keys := make([]string, 0, 2)
	for _, key := range []rune{first, second} {
		if key != 0 {
			keys = append(keys, string(key))
		}
	}
	return strings.Join(keys, "/")
}

// commentActionHints names what a picked card answers to, in the order a reader
// reaches for them. Each key is read back from the bindings and dropped when a
// binding took its rune, so the hint never states a default the user has moved.
func (a *App) commentActionHints() []string {
	labels := make([]string, 0, 4)
	for _, action := range []struct {
		id       string
		fallback rune
		verb     string
	}{
		{"comment_reply", 'r', "reply"},
		{"comment_quote", 'Q', "quote"},
		{"comment_copy_link", 'y', "copy link"},
		{"comment_open", 'o', "open"},
	} {
		if key := a.actionKey(action.id, action.fallback); key != 0 {
			labels = append(labels, fmt.Sprintf("%c %s", key, action.verb))
		}
	}
	return labels
}

// hint is a key and the verb it performs, the unit the status bar prints.
type hint struct {
	key  string
	verb string
}

// hintLine joins hints in reading order, dropping any left keyless because a
// binding took the rune. The hint never states a default the user has moved.
func (a *App) hintLine(hints ...hint) string {
	labels := make([]string, 0, len(hints))
	for _, item := range hints {
		if item.key == "" {
			continue
		}
		labels = append(labels, item.key+" "+item.verb)
	}
	if len(labels) == 0 {
		return ""
	}
	return a.themeTags.SecondaryText + strings.Join(labels, " · ") + "[-]"
}

// actionHint names a UI action's key, or nothing when a binding took the rune.
func (a *App) actionHint(id string, fallback rune, verb string) hint {
	key := a.actionKey(id, fallback)
	if key == 0 {
		return hint{}
	}
	return hint{key: string(key), verb: verb}
}

// commandHint names a command's key, or nothing when the command is reachable
// only from the palette because a binding took its rune.
func (a *App) commandHint(id, verb string) hint {
	key, ok := a.commandShortcutLabel(id)
	if !ok {
		return hint{}
	}
	return hint{key: key, verb: verb}
}

// tabsHint names the pair of keys that step a pane's tabs.
func (a *App) tabsHint() hint {
	return hint{
		key:  keyPairLabel(a.actionKey("tab_prev", '['), a.actionKey("tab_next", ']')),
		verb: "tabs",
	}
}

// updateStatusBar rewrites the pane hints and whatever was last flashed. The
// palette leads every line, being the way to everything the bar has no room to
// name. What list is on screen, how it is sorted and what it is filtered by are
// the issues pane's own footer, not this bar's business.
func (a *App) updateStatusBar() {
	tabs := a.tabsHint()
	view := a.commandHint("zoom_details", "view")
	hideDetails := a.commandHint("toggle_details_pane", "hide details")

	hints := []hint{a.actionHint("open_palette", ':', "palette")}
	note := ""

	switch a.focusedPane {
	case FocusNavigation:
		// The tree is the leftmost pane and stepPane does not wrap, so there is
		// no previous pane to name. h stays with the tree, where it collapses.
		hints = append(hints, hint{"↑↓", "move"}, hint{"⏎", "open"}, hint{"l", "issues"},
			a.commandHint("toggle_navigation_pane", "hide nav"))
	case FocusIssues:
		hints = append(hints, hint{"j/k", "move"}, hint{"⏎", "preview"}, view, tabs, hint{"h/l", "panes"})
	case FocusDetails:
		// The keys a box or a picked card answers to are named on the card
		// itself, in the row with the Post button and in the border under the
		// comment. Saying them again here is the same fact twice, so the bar
		// names only what the card cannot: where the reader is on the page.
		// Read off the field, not live focus: a focus callback can reach here
		// from inside a draw.
		switch {
		case a.commentsFocus != commentsFocusCards && a.detailsCommentsVisible && a.focusedDetailsView:
			// Every key in the box types, the palette's included, so the line
			// names none of them.
			hints = nil
			note = "Writing a comment"
			if a.commentsFocus == commentsFocusReply || a.commentsFocus == commentsFocusReplyPost {
				note = "Writing a reply"
			}
		case a.cardsHaveFocus() && a.focusedCommentID != "":
			hints = append(hints, hint{"Tab", "next comment"}, hint{"Esc", "let go"}, tabs, view, hideDetails)
		case len(a.commentSpans) > 0 && a.cardsHaveFocus():
			hints = append(hints, hint{"j/k", "scroll"}, hint{"Tab", "pick a comment"}, tabs, view, hideDetails)
		case a.detailsZoomed:
			// Below the wide breakpoint the zoom leaves no nav tree to step
			// onto, so offering the key there would be a lie.
			toNav := hint{}
			if a.layoutMode == layoutWide && !a.navigationHidden {
				toNav = hint{"←/h", "navigation"}
			}
			hints = append(hints, hint{"j/k", "scroll"}, tabs, a.commandHint("zoom_details", "exit view"),
				toNav, hint{"Esc", "back to list"})
		default:
			hints = append(hints, hint{"j/k", "scroll"}, tabs, view, hideDetails, hint{"h", "back"})
		}
	case FocusPalette:
		hints = []hint{{"↑↓", "move"}, {"⏎", "run"}, {"Esc", "close"}}
	default:
		hints = append(hints, hint{"j/k", "move"}, hint{"h/l", "panes"})
	}

	text := a.hintLine(hints...)
	if note != "" {
		text += fmt.Sprintf("%s%s[-]", a.themeTags.SecondaryText, note)
	}
	if a.statusMessage != "" {
		text += fmt.Sprintf("%s · [-]%s%s[-]", a.themeTags.Border, a.themeTags.Accent, a.statusMessage)
	}

	a.statusBar.SetText(text)
}

// updateStatusBarWithError updates the status bar with an error message.
func (a *App) updateStatusBarWithError(err error) {
	a.statusBar.SetText(fmt.Sprintf("%sError: %v[-]", a.themeTags.Error, err))
}

func (a *App) flashStatus(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	a.statusMessage = message
	a.statusBar.SetText(fmt.Sprintf("%s%s[-]", a.themeTags.Accent, message))
}
