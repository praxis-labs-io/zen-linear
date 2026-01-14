package tui

import (
	"github.com/rivo/tview"
)

// buildStatusBar creates and configures the status bar widget.
func (a *App) buildStatusBar() *tview.TextView {
	statusBar := tview.NewTextView()
	statusBar.SetDynamicColors(true).
		SetWrap(false).
		SetBorder(false).
		SetBackgroundColor(LinearTheme.HeaderBg) // Use header bg for status bar

	// Add padding
	statusBar.SetBorderPadding(0, 0, 1, 1)

	return statusBar
}
