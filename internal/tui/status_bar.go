package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// buildStatusBar creates and configures the status bar widget.
func (a *App) buildStatusBar() *tview.TextView {
	statusBar := tview.NewTextView()
	statusBar.SetDynamicColors(true).
		SetWrap(false).
		SetBorder(false).
		SetBackgroundColor(a.theme.HeaderBg) // Use header bg for status bar

	// Add padding
	padding := a.density.StatusBarPadding
	statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	return statusBar
}

// keyPairLabel renders two action keys side by side, dropping either one that
// resolved to nothing because a binding took its rune.
func keyPairLabel(first, second rune) string {
	label := ""
	for _, key := range []rune{first, second} {
		if key != 0 {
			label += string(key)
		}
	}
	return label
}

// commentActionHint names what the focused card answers to, each key read back
// from the bindings and dropped when a binding took its rune. The whole hint
// disappears when every one of them has been taken, separator included.
func (a *App) commentActionHint() string {
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
			labels = append(labels, fmt.Sprintf("%c: %s", key, action.verb))
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, " | ") + " | "
}

// zoomHint names the zoom key and the verb it performs, or nothing at all when
// a keybinding has stolen the rune and left the command reachable only from the
// palette. The trailing separator belongs to the hint so it disappears with it.
func (a *App) zoomHint(verb string) string {
	key, ok := a.commandShortcutLabel("zoom_details")
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s: %s | ", key, verb)
}

// updateStatusBar updates the status bar with current information.
func (a *App) updateStatusBar() {
	var helpText string
	keyColor := a.themeTags.SecondaryText

	switch a.focusedPane {
	case FocusNavigation:
		// The tree is the leftmost pane and stepPane does not wrap, so there is
		// no previous pane to name. h stays with the tree, where it collapses.
		helpText = fmt.Sprintf("%s↑↓: navigate | Enter: select | →/l: next pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusIssues:
		helpText = fmt.Sprintf("%sj/k: navigate | Enter: select | →/l: next pane | ←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusDetails:
		// Both keys are remappable, so the hint reads them back rather than
		// stating the defaults at a user who has moved them. A key another
		// binding has taken resolves to 0 and is left out rather than printed.
		tabs := keyPairLabel(a.actionKey("tab_prev", '['), a.actionKey("tab_next", ']'))
		if a.commentsFocus != commentsFocusCards && a.detailsCommentsVisible && a.focusedDetailsView {
			// Every other key in the box is a character in the comment, so the
			// hint names only the ones that are not. Read off the field, not
			// live focus: a focus callback can reach here from inside a draw.
			back := "Esc: back to comments"
			if a.replyParentID() != "" && (a.commentsFocus == commentsFocusReply || a.commentsFocus == commentsFocusReplyPost) {
				back = "Esc: close the reply"
			}
			if a.commentsFocus == commentsFocusPost || a.commentsFocus == commentsFocusReplyPost {
				helpText = fmt.Sprintf("%sEnter: post | %s[-]", keyColor, back)
				break
			}
			helpText = fmt.Sprintf("%sCtrl+Enter: post | Tab: Post button | %s[-]", keyColor, back)
			break
		}
		if a.cardsHaveFocus() && a.focusedCommentID != "" {
			// A lit card owns these keys, and the issue keys they shadow are
			// not worth naming while it does.
			helpText = fmt.Sprintf("%s%sTab: next comment | Esc: let go[-]", keyColor, a.commentActionHint())
			break
		}
		if len(a.commentSpans) > 0 && a.cardsHaveFocus() {
			helpText = fmt.Sprintf("%sj/k, Ctrl+D/U: scroll | Tab: pick a comment | %s: switch description/comments[-]",
				keyColor, tabs)
			break
		}
		if a.detailsZoomed {
			// Below the wide breakpoint the zoom leaves no nav tree to step
			// onto, so offering the key there would be a lie.
			toNav := ""
			if a.layoutMode == layoutWide && !a.navigationHidden {
				toNav = "←/h: navigation | "
			}
			helpText = fmt.Sprintf("%sj/k, Ctrl+D/U: scroll | %s: switch description/comments | %s%sEsc: back to list | :: palette | /: search | q: quit[-]",
				keyColor, tabs, toNav, a.zoomHint("unzoom"))
			break
		}
		helpText = fmt.Sprintf("%sj/k, Ctrl+D/U: scroll | %s: switch description/comments | %s←/h: prev pane | :: palette | /: search | q: quit[-]",
			keyColor, tabs, a.zoomHint("zoom"))
	case FocusPalette:
		helpText = fmt.Sprintf("%s↑↓: navigate | Enter: execute | Esc: close[-]", keyColor)
	default:
		helpText = fmt.Sprintf("%sj/k: navigate | l: next pane | h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	}

	navText := ""
	if a.selectedNavigation != nil {
		label := a.selectedNavigation.Text
		if a.selectedNavigation.IsStatus {
			if a.selectedNavigation.StateName != "" {
				label = fmt.Sprintf("Status: %s", a.selectedNavigation.StateName)
			} else {
				label = "Status"
			}
		} else if a.selectedNavigation.IsCycle {
			if a.selectedNavigation.CycleName != "" {
				label = fmt.Sprintf("Cycle: %s", a.selectedNavigation.CycleName)
			} else {
				label = "Cycle"
			}
		}
		navText = fmt.Sprintf("%s%s[-]", a.themeTags.Accent, label)
	}

	filterText := ""
	if !a.richFilters.Empty() {
		filterText = fmt.Sprintf("%sFilters: %s[-]", a.themeTags.Warning, a.richFilters.Summary())
	}

	a.issuesMu.RLock()
	issuesLen := len(a.issues)
	a.issuesMu.RUnlock()
	statusText := fmt.Sprintf("%s%d issues[-]", a.themeTags.Accent, issuesLen)
	if issuesLen == 0 {
		statusText = fmt.Sprintf("%sNo issues[-]", a.themeTags.SecondaryText)
	}

	sep := fmt.Sprintf("%s | [-]", a.themeTags.Border)

	sortText := fmt.Sprintf("%sSort: %s[-]", a.themeTags.SecondaryText, sortChainLabel(a.effectiveSortFields()))

	parts := []string{helpText}
	if navText != "" {
		parts = append(parts, navText)
	}
	if filterText != "" {
		parts = append(parts, filterText)
	}
	parts = append(parts, sortText)
	if a.statusMessage != "" {
		parts = append(parts, fmt.Sprintf("%s%s[-]", a.themeTags.Accent, a.statusMessage))
	}
	parts = append(parts, statusText)

	text := parts[0]
	for i := 1; i < len(parts); i++ {
		text += sep + parts[i]
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
