package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
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
	a.statusToast.SetText(a.toastTag() + tview.Escape(message) + "[-]")
	a.statusRow.ResizeItem(a.statusToast, width, 0)
}

// toastTag colors the corner by what it is saying. A result carries the theme's
// success or failure color; a nudge is plain text, so those two colors mean
// something when they appear. An empty message means what is showing is the
// loading line, which takes the accent: it is the app working, not a result,
// and the accent is what the rest of the chrome uses to say something is live.
func (a *App) toastTag() string {
	if a.statusMessage == "" {
		return a.themeTags.Accent
	}
	switch a.statusLevel {
	case statusSuccess:
		return a.themeTags.Success
	case statusError:
		return a.themeTags.Error
	default:
		return a.themeTags.Foreground
	}
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
//
// Only the keys that act on the conversation. comment_copy_link and
// comment_open still answer here; they leave for a browser or a clipboard, and
// naming them cost a crowded border to tell a reader something the README
// already does. Edit and delete are yours alone.
func (a *App) commentActionHints(comment linearapi.Comment) []string {
	actions := []commentAction{{"comment_reply", 'r', "reply"}}
	if comment.Author.IsMe {
		actions = append(actions,
			commentAction{"comment_edit", 'e', "edit"},
			commentAction{"comment_delete", 'd', "delete"})
	}
	actions = append(actions, commentAction{"comment_quote", 'Q', "quote"})

	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		if key := a.actionKey(action.id, action.fallback); key != 0 {
			labels = append(labels, fmt.Sprintf("%c %s", key, action.verb))
		}
	}
	return labels
}

// commentAction is one key a card offers: the binding id, the rune it takes
// when nothing else claimed it, and the word the hint prints.
type commentAction struct {
	id       string
	fallback rune
	verb     string
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

// commentsHint names the pair of keys that step the page's comments.
func (a *App) commentsHint() hint {
	return hint{
		key:  keyPairLabel(a.actionKey("comment_prev", '{'), a.actionKey("comment_next", '}')),
		verb: "comments",
	}
}

// updateStatusBar rewrites the pane hints and whatever was last flashed. The
// palette leads every line, being the way to everything the bar has no room to
// name. What list is on screen, how it is sorted and what it is filtered by are
// the issues pane's own footer, not this bar's business.
func (a *App) updateStatusBar() {
	comments := a.commentsHint()
	view := a.commandHint("zoom_details", "view")
	hideDetails := a.commandHint("toggle_details_pane", "hide details")
	// The zoom's own keys, decided here rather than in the branches below: those
	// are picked by what the comment ring is doing, which has nothing to say
	// about the zoom, so a lit card used to leave the reader with hints for a
	// layout they were not in.
	toNav, backToList := hint{}, hint{}
	if a.detailsZoomed {
		// The same key closes the view again, and hiding the pane is inert.
		view = a.commandHint("zoom_details", "close")
		hideDetails = hint{}
		// Below the wide breakpoint the zoom leaves no nav tree to step onto,
		// so offering the key there would be a lie.
		if a.layoutMode == layoutWide && !a.navigationHidden {
			toNav = hint{"←/h", "navigation"}
		}
		backToList = hint{"Esc", "back to list"}
	}

	hints := []hint{a.actionHint("open_palette", ':', "palette")}
	note := ""

	switch a.focusedPane {
	case FocusNavigation:
		// Read the field, not live focus: a focus callback can reach here from
		// inside a draw.
		if a.navSearchFocused {
			// Every letter in the box types, the palette's included, so the
			// line names only the keys that leave it.
			hints = []hint{{"⏎", "results"}, {"↓", "tree"}, {"Esc", "clear"}}
			break
		}
		// The tree is the leftmost pane and stepPane does not wrap, so there is
		// no previous pane to name. h stays with the tree, where it collapses.
		hints = append(hints, hint{"↑↓", "move"}, hint{"⏎", "open"}, hint{"Tab", "search"},
			hint{"l", "issues"}, a.commandHint("toggle_navigation_pane", "hide nav"))
	case FocusIssues:
		hints = append(hints, hint{"j/k", "move"}, hint{"⏎", "preview"}, view,
			a.actionHint("search", '/', "search"), hint{"h/l", "panes"})
	case FocusDetails:
		// The keys a box or a picked card answers to are named on the card
		// itself, in the row with the Post button and in the border under the
		// comment. Saying them again here is the same fact twice, so the bar
		// names only what the card cannot: where the reader is on the page.
		// Read off the field, not live focus: a focus callback can reach here
		// from inside a draw.
		switch {
		case a.detailsEdit.editing == issueFieldDescription:
			// Enter is a newline in prose, so the chord is what sends it.
			hints = []hint{{"⌃⏎", "save"}, {"Esc", "cancel"}}
			note = "Editing " + issueFieldNames[issueFieldDescription]
		case a.detailsEdit.editing != "":
			// Every key in the box types, so the line names only the two it
			// does not get.
			hints = []hint{{"⏎", "save"}, {"Esc", "cancel"}}
			note = "Editing " + issueFieldNames[a.detailsEdit.editing]
		case a.detailsEdit.open == issueFieldLabels:
			// A set rather than a value: the toggles are local until Enter, so
			// the line says apply where the others say set.
			hints = []hint{{"j/k", "option"}, {"space", "toggle"}, {"⏎", "apply"}, {"Esc", "cancel"}}
			note = "Choosing " + issueFieldNames[issueFieldLabels]
		case a.detailsEdit.open != "":
			hints = []hint{{"j/k", "option"}, {"⏎", "set"}, {"Esc", "cancel"}}
			note = "Choosing " + issueFieldNames[a.detailsEdit.open]
		case a.detailsEdit.on:
			// The mode swallows every other key, the palette's included, so the
			// line names only what it answers to.
			hints = []hint{{"j/k", "field"}}
			// Enter is inert on a field with neither a list nor a box behind it,
			// and a hint for a key that does nothing is worse than no hint.
			switch {
			case fieldHasChooser(a.detailsEdit.cursor):
				hints = append(hints, hint{"⏎", "open"})
			case fieldHasEditor(a.detailsEdit.cursor):
				hints = append(hints, hint{"⏎", "edit"})
			}
			hints = append(hints, hint{"Esc", "done"})
			note = "Editing fields"
		case a.detailsFocus != detailsFocusCards && a.detailsHaveFocus():
			// Every key in the box types, the palette's included, so the line
			// names none of them.
			hints = nil
			note = "Writing a comment"
			switch a.detailsFocus {
			case detailsFocusReply, detailsFocusReplyPost:
				note = "Writing a reply"
			case detailsFocusEdit, detailsFocusEditPost:
				note = "Editing a comment"
			}
		case a.cardsHaveFocus() && a.focusedCommentID != "":
			// Esc is a ladder: it lets go of the card first and only the next
			// press leaves the zoom, so the line names the rung it is on and
			// backToList waits its turn.
			hints = append(hints, hint{"Esc", "let go"}, comments, view, hideDetails, toNav)
		case len(a.commentSpans) > 0 && a.cardsHaveFocus():
			hints = append(hints, hint{"j/k", "scroll"}, comments, view, hideDetails, toNav, backToList)
		case a.detailsZoomed:
			hints = append(hints, hint{"j/k", "scroll"}, comments, view, toNav, backToList)
		default:
			hints = append(hints, hint{"j/k", "scroll"}, comments, view, hideDetails, hint{"h", "back"})
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
	a.statusLevel = statusInfo
	a.fitStatusToast()
	a.statusBar.SetText(fmt.Sprintf("%sError: %s[-]", a.themeTags.Error, tview.Escape(err.Error())))
}

// statusLevel is what a flashed message is: something the user should know,
// something that finished, or something that failed.
type statusLevel int

const (
	statusInfo statusLevel = iota
	statusSuccess
	statusError
)

// flashStatus says what just happened, in the strip's right corner, until it
// expires. Plain text: a nudge, a count, a state nobody asked about.
func (a *App) flashStatus(message string) {
	a.flashStatusLevel(statusInfo, message)
}

// flashSuccess says an action finished, in green.
func (a *App) flashSuccess(message string) {
	a.flashStatusLevel(statusSuccess, message)
}

// flashError says an action failed, in red. For a failure that carries an
// error value, updateStatusBarWithError keeps the whole thing on the hint line
// instead: this corner is sized to a few words.
func (a *App) flashError(message string) {
	a.flashStatusLevel(statusError, message)
}

func (a *App) flashStatusLevel(level statusLevel, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	a.statusMessage = message
	a.statusLevel = level
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
			a.statusLevel = statusInfo
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
