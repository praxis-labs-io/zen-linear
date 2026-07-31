package tui

import (
	"fmt"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/gdamore/tcell/v2"
)

// hexPtr converts a tcell color to a glamour hex string. ColorDefault maps to
// nil so the terminal's own color shows through.
func hexPtr(color tcell.Color) *string {
	if !color.Valid() {
		return nil
	}
	hex := fmt.Sprintf("#%06x", color.Hex())
	return &hex
}

func uintPtr(value uint) *uint { return &value }

// themeMarkdownStyle derives a glamour style from the active theme so rendered
// markdown matches the rest of the UI instead of glamour's stock palette. The
// document margin is dropped and wrapping is left to the surrounding text
// view, which knows its own width.
func themeMarkdownStyle(theme Theme) ansi.StyleConfig {
	style := styles.DarkStyleConfig

	style.Document.Color = hexPtr(theme.Foreground)
	style.Document.Margin = uintPtr(0)

	style.BlockQuote.Color = hexPtr(theme.SecondaryText)

	style.Heading.Color = hexPtr(theme.Accent)
	style.H1.Color = hexPtr(theme.InverseTextColor())
	style.H1.BackgroundColor = hexPtr(theme.Accent)
	style.H6.Color = hexPtr(theme.Accent)

	style.HorizontalRule.Color = hexPtr(theme.Border)

	style.Link.Color = hexPtr(theme.StatusDone)
	style.LinkText.Color = hexPtr(theme.Accent)
	style.Image.Color = hexPtr(theme.StatusDone)
	style.ImageText.Color = hexPtr(theme.SecondaryText)

	// Inline code keeps a highlight color but loses the stock background chip,
	// which clashes with themed (and transparent) backgrounds. Code blocks drop
	// chroma's palette for a single theme color for the same reason.
	style.Code.Color = hexPtr(theme.StatusInProgress)
	style.Code.BackgroundColor = nil
	style.CodeBlock.Color = hexPtr(theme.StatusDone)
	style.CodeBlock.Chroma = nil

	return style
}
