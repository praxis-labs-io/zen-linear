package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// flashDuration is how long a one-off message holds its place beside the hints.
// A var so a test does not have to wait it out.
var flashDuration = 4 * time.Second

// statusToastGap is the space kept between the hints and a message, so the two
// halves of the strip never run together.
const statusToastGap = 2

// buildStatusBar builds the strip under the panes: pane hints on the left, a
// flashed message on the right. They are two views rather than one line of text
// so a message lands in the same place whatever the hints are saying.
func (a *App) buildStatusBar() {
	a.statusBar = a.newStatusView(tview.AlignLeft)
	a.statusToast = a.newStatusView(tview.AlignRight)

	a.statusRow = tview.NewFlex()
	// Flex sets dontClear and never paints its own background; restore the fill
	// so the layer beneath cannot bleed through.
	a.statusRow.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.statusRow.
		AddItem(a.statusBar, 0, 1, false).
		AddItem(a.statusToast, 0, 0, false)
	// The corner is sized against the live width here rather than where the
	// message is set, which is the only place that width is known.
	a.statusRow.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		a.statusRowWidth = width
		a.fitStatusToast()
		return x, y, width, height
	})

	a.applyStatusBarPadding()
}

// newStatusView returns one half of the strip. The text style carries the
// background as well: tview repaints a TextView's inner rect in its text style
// whenever that differs from the box, which left the padding columns as blocks
// in a second color.
func (a *App) newStatusView(align int) *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(align).
		SetTextStyle(tcell.StyleDefault.Background(a.theme.Background).Foreground(a.theme.SecondaryText))
	view.SetBorder(false).SetBackgroundColor(a.theme.Background)
	return view
}

// applyStatusBarPadding keeps the strip's gaps at its outer edges, so the two
// views sit flush against each other in the middle.
func (a *App) applyStatusBarPadding() {
	padding := a.density.StatusBarPadding
	a.statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, 0)
	a.statusToast.SetBorderPadding(padding.Top, padding.Bottom, 0, padding.Right)
}

// fitStatusToast puts the flashed message in the right of the strip, sizing its
// corner to the words and closing it up when there is nothing to say. Half the
// row is the ceiling: a fixed half wider than the strip leaves the hints a
// negative width, which tview draws from, and one long message would take the
// whole line with it. Before the first draw the width is unknown and the
// message is sized as it stands; the draw that follows corrects it.
func (a *App) fitStatusToast() {
	message := a.statusMessage
	if message == "" {
		message = a.loadingMessage
	}
	gap := a.density.StatusBarPadding.Right + statusToastGap

	width := 0
	if message != "" {
		if a.statusRowWidth > 0 {
			message = runewidth.Truncate(message, max(0, a.statusRowWidth/2-gap), "…")
		}
		width = runewidth.StringWidth(message) + gap
	}
	a.statusToast.SetText(a.themeTags.Accent + tview.Escape(message) + "[-]")
	a.statusRow.ResizeItem(a.statusToast, width, 0)
}

// setLoadingMessage says what a fetch is doing, in the same corner and behind
// anything flashed: progress repeats itself and a warning does not, so the
// warning is the one that must not be pushed off. Empty clears it. UI thread
// only, like everything else the bar reads.
func (a *App) setLoadingMessage(message string) {
	a.loadingMessage = message
	a.fitStatusToast()
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
// The key is lit and the verb is not, so the keys read as a column of their own
// down the line.
func (a *App) hintLine(hints ...hint) string {
	labels := make([]string, 0, len(hints))
	for _, item := range hints {
		if item.key == "" {
			continue
		}
		labels = append(labels, a.themeTags.Accent+tview.Escape(item.key)+"[-]"+a.themeTags.SecondaryText+" "+item.verb)
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

	a.statusBar.SetText(text)
	a.fitStatusToast()
}

// updateStatusBarWithError leaves a failure on the bar until something else
// takes the bar. It sits with the hints rather than in the message's corner,
// which is sized to a few words. The text is escaped: the view reads color
// tags, and a Linear error carrying a bracketed fragment would lose the part
// naming what failed. A flash still counting down is dropped, or its clear
// would repaint over this on its own clock.
func (a *App) updateStatusBarWithError(err error) {
	a.cancelStatusFlash()
	a.statusMessage = ""
	a.fitStatusToast()
	a.statusBar.SetText(fmt.Sprintf("%sError: %s[-]", a.themeTags.Error, tview.Escape(err.Error())))
}

// flashStatus says what just happened, in the strip's right corner, until it
// expires.
func (a *App) flashStatus(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	a.statusMessage = message
	a.updateStatusBar()
	a.scheduleStatusFlashClear()
}

// scheduleStatusFlashClear takes the message back down after flashDuration. The
// generation is what keeps a stale timer from clearing a newer message.
func (a *App) scheduleStatusFlashClear() {
	generation := a.statusFlashGeneration.Add(1)

	a.statusFlashMu.Lock()
	defer a.statusFlashMu.Unlock()
	if a.statusFlashTimer != nil {
		a.statusFlashTimer.Stop()
	}
	a.statusFlashTimer = time.AfterFunc(flashDuration, func() {
		if generation != a.statusFlashGeneration.Load() {
			return
		}
		a.QueueUpdateDraw(func() {
			if generation != a.statusFlashGeneration.Load() {
				return
			}
			a.statusMessage = ""
			a.updateStatusBar()
		})
	})
}

// cancelStatusFlash drops a clear still pending. The timer only: the message
// itself is UI state, written on the event loop, and this is also called from
// teardown after that loop has stopped.
func (a *App) cancelStatusFlash() {
	a.statusFlashGeneration.Add(1)

	a.statusFlashMu.Lock()
	defer a.statusFlashMu.Unlock()
	if a.statusFlashTimer != nil {
		a.statusFlashTimer.Stop()
		a.statusFlashTimer = nil
	}
}
