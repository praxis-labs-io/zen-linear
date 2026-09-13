package tui

import (
	"math"

	"github.com/gdamore/tcell/v2"
)

const (
	ansiBlack       = tcell.ColorBlack
	ansiRed         = tcell.ColorMaroon
	ansiGreen       = tcell.ColorGreen
	ansiYellow      = tcell.ColorOlive
	ansiBlue        = tcell.ColorNavy
	ansiMagenta     = tcell.ColorPurple
	ansiWhite       = tcell.ColorSilver
	ansiBrightBlack = tcell.ColorGray
)

type terminalSurface struct {
	background tcell.Color
	foreground tcell.Color
	known      bool
}

var detectedSurface terminalSurface

var detectedKittyGraphics bool

// DetectTerminalCapabilities queries the terminal once. Call it at launch, before tcell owns the tty.
func DetectTerminalCapabilities() {
	reply := queryTerminal()
	detectedSurface = terminalSurface{
		background: reply.background,
		foreground: reply.foreground,
		known:      reply.colorsKnown,
	}
	detectedKittyGraphics = reply.kittyGraphics
}

// KittyGraphicsSupported is false until DetectTerminalCapabilities has run, and always off unix.
func KittyGraphicsSupported() bool {
	return detectedKittyGraphics
}

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
