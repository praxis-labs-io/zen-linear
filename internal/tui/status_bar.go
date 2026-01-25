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
		SetBackgroundColor(a.theme.HeaderBg) // Use header bg for status bar

	// Add padding
	padding := a.density.StatusBarPadding
	statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	return statusBar
}
