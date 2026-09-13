package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/gdamore/tcell/v2"
)

func hexPtr(color tcell.Color) *string {
	if !color.Valid() {
		return nil
	}
	if !color.IsRGB() {
		index := strconv.Itoa(int(color &^ tcell.ColorValid))
		return &index
	}
	hex := fmt.Sprintf("#%06x", color.Hex())
	return &hex
}

func uintPtr(value uint) *uint { return &value }

func boolPtr(value bool) *bool { return &value }

// Links carry no underline: tview's [-:-:-] reset leaves tcell's underline style set, so it would run to the end of the pane.
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
	style.Link.Underline = boolPtr(false)
	style.LinkText.Color = hexPtr(theme.Accent)
	style.Image.Color = hexPtr(theme.StatusDone)
	style.Image.Underline = boolPtr(false)
	style.ImageText.Color = hexPtr(theme.SecondaryText)

	style.Code.Color = hexPtr(theme.StatusInProgress)
	style.Code.BackgroundColor = nil
	style.CodeBlock.Color = hexPtr(theme.StatusDone)
	style.CodeBlock.Chroma = nil

	return style
}
