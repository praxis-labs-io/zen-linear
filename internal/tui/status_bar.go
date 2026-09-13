package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

// A var so a test does not have to wait it out.
var flashDuration = 4 * time.Second

const statusToastGap = 2

func (a *App) buildStatusBar() {
	a.statusBar = a.newStatusView(tview.AlignLeft)
	a.statusToast = a.newStatusView(tview.AlignRight)

	a.statusRow = tview.NewFlex()
	a.statusRow.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.statusRow.
		AddItem(a.statusBar, 0, 1, false).
		AddItem(a.statusToast, 0, 0, false)
	a.statusRow.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		a.statusRowWidth = width
		a.fitStatusToast()
		return x, y, width, height
	})

	a.applyStatusBarPadding()
}

// The text style carries the background, or tview repaints the padding columns in a second color.
func (a *App) newStatusView(align int) *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(align).
		SetTextStyle(tcell.StyleDefault.Background(a.theme.Background).Foreground(a.theme.SecondaryText))
	view.SetBorder(false).SetBackgroundColor(a.theme.Background)
	return view
}

func (a *App) applyStatusBarPadding() {
	padding := a.density.StatusBarPadding
	a.statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, 0)
	a.statusToast.SetBorderPadding(padding.Top, padding.Bottom, 0, padding.Right)
}

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

func (a *App) setLoadingMessage(message string) {
	a.loadingMessage = message
	a.fitStatusToast()
}

func keyPairLabel(first, second rune) string {
	keys := make([]string, 0, 2)
	for _, key := range []rune{first, second} {
		if key != 0 {
			keys = append(keys, string(key))
		}
	}
	return strings.Join(keys, "/")
}

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

type commentAction struct {
	id       string
	fallback rune
	verb     string
}

type hint struct {
	key  string
	verb string
}

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

func (a *App) actionHint(id string, fallback rune, verb string) hint {
	key := a.actionKey(id, fallback)
	if key == 0 {
		return hint{}
	}
	return hint{key: string(key), verb: verb}
}

func (a *App) commandHint(id, verb string) hint {
	key, ok := a.commandShortcutLabel(id)
	if !ok {
		return hint{}
	}
	return hint{key: key, verb: verb}
}

func (a *App) commentsHint() hint {
	return hint{
		key:  keyPairLabel(a.actionKey("comment_prev", '{'), a.actionKey("comment_next", '}')),
		verb: "comments",
	}
}

func (a *App) updateStatusBar() {
	comments := a.commentsHint()
	view := a.commandHint("zoom_details", "view")
	hideDetails := a.commandHint("toggle_details_pane", "hide details")
	toNav, backToList := hint{}, hint{}
	if a.detailsZoomed {
		view = a.commandHint("zoom_details", "close")
		hideDetails = hint{}
		if a.layoutMode == layoutWide && !a.navigationHidden {
			toNav = hint{"←/h", "navigation"}
		}
		backToList = hint{"Esc", "back to list"}
	}

	hints := []hint{a.actionHint("open_palette", ':', "palette")}
	note := ""

	switch a.focusedPane {
	case FocusNavigation:
		if a.navSearchFocused {
			hints = []hint{{"⏎", "results"}, {"↓", "tree"}, {"Esc", "clear"}}
			break
		}
		hints = append(hints, hint{"↑↓", "move"}, hint{"⏎", "open"}, hint{"Tab", "search"},
			hint{"l", "issues"}, a.commandHint("toggle_navigation_pane", "hide nav"))
	case FocusIssues:
		hints = append(hints, hint{"j/k", "move"}, hint{"⏎", "preview"}, view,
			a.actionHint("search", '/', "search"), hint{"h/l", "panes"})
	case FocusDetails:
		switch {
		case a.detailsEdit.editing == issueFieldDescription:
			hints = []hint{{"⌃S", "save"}, {"Esc", "cancel"}}
			note = "Editing " + issueFieldNames[issueFieldDescription]
		case a.detailsEdit.editing != "":
			hints = []hint{{"⏎", "save"}, {"Esc", "cancel"}}
			note = "Editing " + issueFieldNames[a.detailsEdit.editing]
		case a.detailsEdit.open == issueFieldLabels:
			hints = []hint{{"j/k", "option"}, {"space", "toggle"}, {"⏎", "apply"}, {"Esc", "cancel"}}
			note = "Choosing " + issueFieldNames[issueFieldLabels]
		case a.detailsEdit.open != "":
			hints = []hint{{"j/k", "option"}, {"⏎", "set"}, {"Esc", "cancel"}}
			note = "Choosing " + issueFieldNames[a.detailsEdit.open]
		case a.detailsEdit.on:
			hints = []hint{{"j/k", "field"}}
			switch {
			case fieldHasChooser(a.detailsEdit.cursor):
				hints = append(hints, hint{"⏎", "open"})
			case fieldHasEditor(a.detailsEdit.cursor):
				hints = append(hints, hint{"⏎", "edit"})
			}
			hints = append(hints, hint{"Esc", "done"})
			note = "Editing fields"
		case a.detailsFocus != detailsFocusCards && a.detailsHaveFocus():
			hints = nil
			note = "Writing a comment"
			switch a.detailsFocus {
			case detailsFocusReply, detailsFocusReplyPost:
				note = "Writing a reply"
			case detailsFocusEdit, detailsFocusEditPost:
				note = "Editing a comment"
			}
		case a.cardsHaveFocus() && a.focusedCommentID != "":
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

func (a *App) updateStatusBarWithError(err error) {
	a.cancelStatusFlash()
	a.statusMessage = ""
	a.statusLevel = statusInfo
	a.fitStatusToast()
	a.statusBar.SetText(fmt.Sprintf("%sError: %s[-]", a.themeTags.Error, tview.Escape(err.Error())))
}

func (a *App) updateStatusBarWithNotice(notice string) {
	a.cancelStatusFlash()
	a.statusMessage = ""
	a.statusLevel = statusInfo
	a.fitStatusToast()
	a.statusBar.SetText(fmt.Sprintf("%s%s[-]", a.themeTags.SecondaryText, tview.Escape(notice)))
}

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusSuccess
	statusError
)

func (a *App) flashStatus(message string) {
	a.flashStatusLevel(statusInfo, message)
}

func (a *App) flashSuccess(message string) {
	a.flashStatusLevel(statusSuccess, message)
}

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

func (a *App) cancelStatusFlash() {
	a.statusFlashGeneration.Add(1)

	a.statusFlashMu.Lock()
	defer a.statusFlashMu.Unlock()
	if a.statusFlashTimer != nil {
		a.statusFlashTimer.Stop()
		a.statusFlashTimer = nil
	}
}
