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

// updateStatusBar updates the status bar with current information.
func (a *App) updateStatusBar() {
	var helpText string
	keyColor := a.themeTags.SecondaryText

	switch a.focusedPane {
	case FocusNavigation:
		helpText = fmt.Sprintf("%s↑↓: navigate | Enter: select | Tab/→/l: next pane | Shift+Tab/←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusIssues:
		helpText = fmt.Sprintf("%sj/k: navigate | Enter: select | Tab/→/l: next pane | Shift+Tab/←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusDetails:
		// Both keys are remappable, so the hint reads them back rather than
		// stating the defaults at a user who has moved them.
		tabs := fmt.Sprintf("%c%c", a.actionKey("tab_prev", '{'), a.actionKey("tab_next", '}'))
		zoom := a.commandShortcutLabel("zoom_details")
		if a.detailsZoomed {
			// Below the wide breakpoint the zoom leaves no nav tree to step
			// onto, so offering the key there would be a lie.
			toNav := ""
			if a.layoutMode == layoutWide && !a.navigationHidden {
				toNav = "←/h: navigation | "
			}
			helpText = fmt.Sprintf("%sj/k, Ctrl+D/U: scroll | %s: switch description/comments | %s%s: unzoom | Esc: back to list | :: palette | /: search | q: quit[-]", keyColor, tabs, toNav, zoom)
			break
		}
		helpText = fmt.Sprintf("%sj/k, Ctrl+D/U: scroll | %s: switch description/comments | %s: zoom | →/l: next pane | Shift+Tab/←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor, tabs, zoom)
	case FocusPalette:
		helpText = fmt.Sprintf("%s↑↓: navigate | Enter: execute | Esc: close[-]", keyColor)
	default:
		helpText = fmt.Sprintf("%sj/k: navigate | Tab: next pane | Shift+Tab: prev pane | :: palette | /: search | q: quit[-]", keyColor)
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
