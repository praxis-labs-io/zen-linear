package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildPaletteModal creates and configures the command palette modal overlay.
func (a *App) buildPaletteModal() *tview.Flex {
	// Create input field for query with improved styling
	a.paletteInput = tview.NewInputField()
	a.paletteInput.
		SetLabel("> ").
		SetLabelColor(LinearTheme.Accent).
		SetFieldWidth(0). // Use full available width
		SetPlaceholder("Type to filter commands...").
		SetPlaceholderTextColor(LinearTheme.SecondaryText).
		SetFieldBackgroundColor(tcell.NewRGBColor(40, 40, 50)). // Slightly lighter than HeaderBg for contrast
		SetFieldTextColor(tcell.ColorWhite).
		SetBackgroundColor(LinearTheme.HeaderBg)

	// Create list for filtered commands
	a.paletteList = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(LinearTheme.Accent).
		SetSelectedTextColor(tcell.ColorWhite).
		SetHighlightFullLine(true)
	a.paletteList.SetBackgroundColor(LinearTheme.HeaderBg)

	// Create help text with better formatting
	helpText := tview.NewTextView()
	helpText.SetText("↑↓ Navigate  •  Enter Execute  •  Esc Close").
		SetTextColor(LinearTheme.SecondaryText).
		SetBackgroundColor(LinearTheme.HeaderBg)
	helpText.SetTextAlign(tview.AlignCenter)

	// Build modal content with better spacing
	// Add a small spacer before input for visual breathing room
	spacerTop := tview.NewBox().SetBackgroundColor(LinearTheme.HeaderBg)
	spacerBottom := tview.NewBox().SetBackgroundColor(LinearTheme.HeaderBg)

	a.paletteModalContent = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(spacerTop, 1, 0, false).
		AddItem(a.paletteInput, 1, 0, true).
		AddItem(a.paletteList, 0, 1, false).
		AddItem(spacerBottom, 1, 0, false).
		AddItem(helpText, 1, 0, false)
	// Set border and styling - must be set after creating the flex but before adding to parent
	a.paletteModalContent.
		SetBackgroundColor(LinearTheme.HeaderBg).
		SetBorder(true).
		SetBorderColor(LinearTheme.Accent).
		SetBorderPadding(0, 0, 0, 0). // No padding - content uses full width
		SetTitle(" Commands ").
		SetTitleColor(LinearTheme.Accent) // Use accent color for title to match border

	// Center the modal on screen with wider width (60 instead of 50) for better readability
	centeredContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(a.paletteModalContent, 15, 0, true).
		AddItem(nil, 0, 1, false)

	horizontalCentered := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(centeredContent, 60, 0, true).
		AddItem(nil, 0, 1, false)

	// Use darker background to create dimming effect (darker than main background)
	modalBg := tcell.NewRGBColor(10, 10, 10) // Very dark gray, darker than Background for contrast
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(horizontalCentered, 0, 1, true).
		AddItem(nil, 0, 1, false)
	modal.SetBackgroundColor(modalBg)

	return modal
}

// updatePaletteList updates the palette list with filtered commands.
func (a *App) updatePaletteList() {
	a.paletteList.Clear()
	filtered := a.paletteCtrl.Filtered()
	cursor := a.paletteCtrl.Cursor()

	// Add all filtered commands to the list with shortcut hints
	// Format: [shortcut] Command Title - with shortcut right-aligned in a fixed column
	for _, cmd := range filtered {
		var shortcutHint string
		if cmd.ShortcutDisplay != "" {
			// Use custom display text (e.g., "/" or "Esc")
			shortcutHint = cmd.ShortcutDisplay
		} else if cmd.ShortcutRune != 0 {
			shortcutHint = FormatShortcut(cmd.ShortcutRune)
		}
		var displayText string
		if shortcutHint != "" {
			// Use fixed width shortcut column (8 chars) followed by command title
			displayText = fmt.Sprintf("[#787878]%8s[-]  %s", shortcutHint, cmd.Title)
		} else {
			// No shortcut - pad with spaces for alignment
			displayText = fmt.Sprintf("[#787878]%8s[-]  %s", "", cmd.Title)
		}
		a.paletteList.AddItem(displayText, "", 0, nil)
	}

	// Set selected item to match cursor position
	if len(filtered) > 0 {
		if cursor >= len(filtered) {
			cursor = len(filtered) - 1
		}
		if cursor < 0 {
			cursor = 0
		}
		a.paletteList.SetCurrentItem(cursor)
	}

	// Update modal to show all commands without scrolling
	// Calculate content height: spacer (1) + input (1) + commands (len(filtered)) + spacer (1) + help (1) = len(filtered) + 4
	contentHeight := len(filtered) + 4
	if contentHeight < 6 {
		contentHeight = 6 // Minimum height for usability
	}
	// Border adds 2 lines (top and bottom), so total height = contentHeight + 2
	requiredHeight := contentHeight + 2

	// Rebuild modalContent with correct list height
	// Create help text with improved formatting
	helpText := tview.NewTextView()
	helpText.SetText("↑↓ Navigate  •  Enter Execute  •  Esc Close").
		SetTextColor(LinearTheme.SecondaryText).
		SetBackgroundColor(LinearTheme.HeaderBg)
	helpText.SetTextAlign(tview.AlignCenter)

	// Add spacers for visual breathing room
	spacerTop := tview.NewBox().SetBackgroundColor(LinearTheme.HeaderBg)
	spacerBottom := tview.NewBox().SetBackgroundColor(LinearTheme.HeaderBg)

	// Rebuild modalContent with list height set to number of commands (no scrolling)
	a.paletteModalContent = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(spacerTop, 1, 0, false).
		AddItem(a.paletteInput, 1, 0, true).
		AddItem(a.paletteList, len(filtered), 0, false).
		AddItem(spacerBottom, 1, 0, false).
		AddItem(helpText, 1, 0, false)
	// Set border and styling - must be set after creating the flex but before adding to parent
	a.paletteModalContent.
		SetBackgroundColor(LinearTheme.HeaderBg).
		SetBorder(true).
		SetBorderColor(LinearTheme.Accent).
		SetBorderPadding(0, 0, 0, 0). // No padding - content uses full width
		SetTitle(" Commands ").
		SetTitleColor(LinearTheme.Accent) // Use accent color for title to match border

	// Rebuild the entire modal with updated height (including border)
	centeredContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(a.paletteModalContent, requiredHeight, 0, true).
		AddItem(nil, 0, 1, false)

	horizontalCentered := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(centeredContent, 60, 0, true).
		AddItem(nil, 0, 1, false)

	// Use darker background to create dimming effect (darker than main background)
	modalBg := tcell.NewRGBColor(10, 10, 10) // Very dark gray, darker than Background for contrast
	a.paletteModal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(horizontalCentered, 0, 1, true).
		AddItem(nil, 0, 1, false)
	a.paletteModal.SetBackgroundColor(modalBg)

	// Replace the modal in pages
	a.pages.RemovePage("palette")
	a.pages.AddPage("palette", a.paletteModal, true, false)
	if a.focusedPane == FocusPalette {
		a.pages.ShowPage("palette")
		a.pages.SendToFront("palette")
	}
}
