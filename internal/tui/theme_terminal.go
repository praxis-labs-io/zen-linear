package tui

import (
	"math"

	"github.com/gdamore/tcell/v2"
)

// The terminal's own palette slots. Named for the role a palette gives them,
// since what they actually paint is the user's business.
const (
	ansiBlack       = tcell.ColorBlack  // 0
	ansiRed         = tcell.ColorMaroon // 1
	ansiGreen       = tcell.ColorGreen  // 2
	ansiYellow      = tcell.ColorOlive  // 3
	ansiBlue        = tcell.ColorNavy   // 4
	ansiMagenta     = tcell.ColorPurple // 5
	ansiWhite       = tcell.ColorSilver // 7
	ansiBrightBlack = tcell.ColorGray   // 8
)

// terminalSurface is the terminal's own background and foreground. known is
// false when nothing answered, which is what the fallback shades are for.
type terminalSurface struct {
	background tcell.Color
	foreground tcell.Color
	known      bool
}

var detectedSurface terminalSurface

// DetectTerminalColors reads the terminal's own colors, once, at launch. A
// query after tcell owns the tty would read the keyboard out from under it.
func DetectTerminalColors() {
	background, foreground, ok := queryTerminalColors()
	detectedSurface = terminalSurface{background: background, foreground: foreground, known: ok}
}

// TerminalTheme is built from the terminal: ANSI slots for the hues, the
// detected background and foreground blended for the shades between them.
func TerminalTheme() Theme {
	return buildTerminalTheme(detectedSurface)
}

func buildTerminalTheme(surface terminalSurface) Theme {
	theme := Theme{
		Background:       tcell.ColorDefault,
		Foreground:       tcell.ColorDefault,
		SelectionText:    tcell.ColorDefault,
		Accent:           ansiBlue,
		BorderFocus:      ansiBlue,
		AssigneeText:     ansiYellow,
		Success:          ansiGreen,
		StatusTriage:     ansiMagenta,
		StatusTodo:       ansiBrightBlack,
		StatusInProgress: ansiYellow,
		StatusReview:     ansiGreen,
		StatusDone:       ansiBlue,
		StatusCanceled:   ansiRed,
	}

	if !surface.known {
		theme.HeaderBg = tcell.ColorDefault
		theme.InputBg = tcell.ColorDefault
		theme.SelectionBg = ansiBrightBlack
		theme.Border = ansiBrightBlack
		theme.HeaderText = ansiWhite
		theme.SecondaryText = ansiBrightBlack
		theme.InverseText = ansiBlack
		return theme
	}

	background, foreground := surface.background, surface.foreground
	theme.HeaderBg = mixColors(background, foreground, 0.06)
	theme.InputBg = mixColors(background, foreground, 0.08)
	theme.SelectionBg = mixColors(background, foreground, 0.12)
	theme.Border = mixColors(background, foreground, 0.30)
	theme.HeaderText = mixColors(foreground, background, 0.25)
	theme.SecondaryText = mixColors(foreground, background, 0.45)
	theme.InverseText = background
	return theme
}

// mixColors blends base toward target. Mixing against the real pair is what
// darkens a light terminal's shades and lightens a dark one's.
func mixColors(base, target tcell.Color, ratio float64) tcell.Color {
	baseRed, baseGreen, baseBlue := base.RGB()
	targetRed, targetGreen, targetBlue := target.RGB()
	return tcell.NewRGBColor(
		mixChannel(baseRed, targetRed, ratio),
		mixChannel(baseGreen, targetGreen, ratio),
		mixChannel(baseBlue, targetBlue, ratio),
	)
}

func mixChannel(base, target int32, ratio float64) int32 {
	return int32(math.Round(float64(base) + (float64(target)-float64(base))*ratio))
}
