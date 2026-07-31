package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// paletteMaxVisibleRows caps how many commands the palette shows at once;
// longer lists scroll.
const paletteMaxVisibleRows = 12

// newThemedInputField creates an InputField whose inner text area fills with
// the given background. tview captures the global primitive background at
// construction time and offers no setter for it afterwards, so without this
// the field row renders in the default (possibly transparent) background with
// color chips behind only the typed text.
func newThemedInputField(fill tcell.Color) *tview.InputField {
	previous := tview.Styles.PrimitiveBackgroundColor
	tview.Styles.PrimitiveBackgroundColor = fill
	field := tview.NewInputField()
	tview.Styles.PrimitiveBackgroundColor = previous
	return field
}

// buildPaletteModal creates and configures the command palette modal overlay.
func (a *App) buildPaletteModal() *tview.Flex {
	// Create input field for query with improved styling
	a.paletteInput = newThemedInputField(a.theme.InputBg)
	a.paletteInput.
		SetLabel("> ").
		SetLabelColor(a.theme.Accent).
		SetFieldWidth(0). // Use full available width
		SetPlaceholder("Type to filter commands...").
		SetFieldStyle(tcell.StyleDefault.Foreground(a.theme.Foreground).Background(a.theme.InputBg)).
		SetPlaceholderStyle(tcell.StyleDefault.Foreground(a.theme.SecondaryText).Background(a.theme.InputBg)).
		SetBackgroundColor(a.theme.ModalBackground())

	// Create list for filtered commands
	a.paletteList = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.Foreground(a.theme.Foreground).Background(a.theme.ModalBackground())).
		SetSelectedStyle(a.listSelectionStyle()).
		SetHighlightFullLine(true)
	a.paletteList.SetBackgroundColor(a.theme.ModalBackground())

	// Create help text with better formatting
	helpText := tview.NewTextView()
	helpText.SetText("↑↓ Navigate  •  Enter Execute  •  Esc Close").
		SetTextColor(a.theme.SecondaryText).
		SetBackgroundColor(a.theme.ModalBackground())
	helpText.SetTextAlign(tview.AlignCenter)

	// Build modal content with better spacing
	// Add a small spacer before input for visual breathing room
	spacerTop := tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())
	spacerBottom := tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())

	a.paletteModalContent = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(spacerTop, a.density.PaletteSpacerLines, 0, false).
		AddItem(a.paletteInput, 1, 0, true).
		AddItem(a.paletteList, 0, 1, false).
		AddItem(spacerBottom, a.density.PaletteSpacerLines, 0, false).
		AddItem(helpText, 1, 0, false)
	a.paletteModalContent.Box = tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())
	// Set border and styling - must be set after creating the flex but before adding to parent
	a.paletteModalContent.
		SetBackgroundColor(a.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(a.theme.Accent).
		SetBorderPadding(0, 0, 0, 0). // No padding - content uses full width
		SetTitle(" Commands ").
		SetTitleColor(a.theme.Accent) // Use accent color for title to match border

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
	modalBg := a.theme.Background
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
			displayText = fmt.Sprintf("%s%8s[-]  %s", a.themeTags.SecondaryText, shortcutHint, cmd.Title)
		} else {
			// No shortcut - pad with spaces for alignment
			displayText = fmt.Sprintf("%s%8s[-]  %s", a.themeTags.SecondaryText, "", cmd.Title)
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

	// Keep the palette compact like a typical command palette: show at most
	// paletteMaxVisibleRows commands (fewer on short screens) and scroll the
	// rest.
	listRows := len(filtered)
	if listRows > paletteMaxVisibleRows {
		listRows = paletteMaxVisibleRows
	}
	chromeRows := 2 + (2 * a.density.PaletteSpacerLines) + 2 // input + help + spacers + border
	if _, _, _, pagesHeight := a.pages.GetRect(); pagesHeight > chromeRows+4 {
		if maxRows := pagesHeight - chromeRows - 2; listRows > maxRows {
			listRows = maxRows
		}
	}
	contentHeight := listRows + 2 + (2 * a.density.PaletteSpacerLines)
	if contentHeight < 6 {
		contentHeight = 6 // Minimum height for usability
	}
	// Border adds 2 lines (top and bottom), so total height = contentHeight + 2
	requiredHeight := contentHeight + 2

	// Rebuild modalContent with correct list height
	// Create help text with improved formatting
	helpText := tview.NewTextView()
	helpText.SetText("↑↓ Navigate  •  Enter Execute  •  Esc Close").
		SetTextColor(a.theme.SecondaryText).
		SetBackgroundColor(a.theme.ModalBackground())
	helpText.SetTextAlign(tview.AlignCenter)

	// Add spacers for visual breathing room
	spacerTop := tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())
	spacerBottom := tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())

	// Rebuild modalContent with the capped list height
	a.paletteModalContent = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(spacerTop, a.density.PaletteSpacerLines, 0, false).
		AddItem(a.paletteInput, 1, 0, true).
		AddItem(a.paletteList, listRows, 0, false).
		AddItem(spacerBottom, a.density.PaletteSpacerLines, 0, false).
		AddItem(helpText, 1, 0, false)
	a.paletteModalContent.Box = tview.NewBox().SetBackgroundColor(a.theme.ModalBackground())
	// Set border and styling - must be set after creating the flex but before adding to parent
	a.paletteModalContent.
		SetBackgroundColor(a.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(a.theme.Accent).
		SetBorderPadding(0, 0, 0, 0). // No padding - content uses full width
		SetTitle(" Commands ").
		SetTitleColor(a.theme.Accent) // Use accent color for title to match border

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
	modalBg := a.theme.Background
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
