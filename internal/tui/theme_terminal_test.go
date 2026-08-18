package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestParseTerminalColorsReadsBothReports(t *testing.T) {
	tests := []struct {
		name       string
		reply      string
		background tcell.Color
		foreground tcell.Color
		ok         bool
	}{
		{
			name:       "four digit components, ST terminated",
			reply:      "\x1b]10;rgb:d0d0/d0d0/d0d0\x1b\\\x1b]11;rgb:1c1c/1c1c/1c1c\x1b\\",
			background: tcell.NewRGBColor(28, 28, 28),
			foreground: tcell.NewRGBColor(208, 208, 208),
			ok:         true,
		},
		{
			name:       "two digit components, BEL terminated",
			reply:      "\x1b]11;rgb:ff/fa/f0\a\x1b]10;rgb:20/20/20\a",
			background: tcell.NewRGBColor(255, 250, 240),
			foreground: tcell.NewRGBColor(32, 32, 32),
			ok:         true,
		},
		{
			name:  "only the background answered",
			reply: "\x1b]11;rgb:1c1c/1c1c/1c1c\x1b\\",
			ok:    false,
		},
		{
			name:  "nothing answered",
			reply: "",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			background, foreground, ok := parseTerminalColors(tt.reply)
			if ok != tt.ok {
				t.Fatalf("parseTerminalColors ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if background != tt.background {
				t.Errorf("background = %s, want %s", colorTag(background), colorTag(tt.background))
			}
			if foreground != tt.foreground {
				t.Errorf("foreground = %s, want %s", colorTag(foreground), colorTag(tt.foreground))
			}
		})
	}
}

// The terminal keeps its own background and its own text color: the theme is
// the shades between them, not a surface painted over them.
func TestTerminalThemeLeavesTheTerminalsOwnColors(t *testing.T) {
	theme := buildTerminalTheme(terminalSurface{
		background: tcell.NewRGBColor(0, 0, 0),
		foreground: tcell.NewRGBColor(255, 255, 255),
		known:      true,
	})

	if theme.Background != tcell.ColorDefault {
		t.Errorf("Background = %s, want the terminal default", colorTag(theme.Background))
	}
	if theme.Foreground != tcell.ColorDefault {
		t.Errorf("Foreground = %s, want the terminal default", colorTag(theme.Foreground))
	}
	if want := tcell.NewRGBColor(31, 31, 31); theme.SelectionBg != want {
		t.Errorf("SelectionBg = %s, want %s", colorTag(theme.SelectionBg), colorTag(want))
	}
	if want := tcell.NewRGBColor(77, 77, 77); theme.Border != want {
		t.Errorf("Border = %s, want %s", colorTag(theme.Border), colorTag(want))
	}
}

// The blend direction is the whole of light-terminal support: the same ratios
// have to darken a light background and lighten a dark one.
func TestTerminalThemeShadesFollowTheBackground(t *testing.T) {
	tests := []struct {
		name       string
		background tcell.Color
		foreground tcell.Color
		darker     bool
	}{
		{"dark terminal", tcell.NewRGBColor(28, 28, 28), tcell.NewRGBColor(220, 220, 220), false},
		{"light terminal", tcell.NewRGBColor(253, 246, 227), tcell.NewRGBColor(60, 60, 60), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := buildTerminalTheme(terminalSurface{background: tt.background, foreground: tt.foreground, known: true})
			selection, _, _ := theme.SelectionBg.RGB()
			background, _, _ := tt.background.RGB()
			if darker := selection < background; darker != tt.darker {
				t.Errorf("SelectionBg %s against background %s: darker = %v, want %v",
					colorTag(theme.SelectionBg), colorTag(tt.background), darker, tt.darker)
			}

			secondary, _, _ := theme.SecondaryText.RGB()
			foreground, _, _ := tt.foreground.RGB()
			if (secondary > foreground) != tt.darker {
				t.Errorf("SecondaryText %s does not sit between the foreground %s and the background",
					colorTag(theme.SecondaryText), colorTag(tt.foreground))
			}
		})
	}
}

// A terminal that never answered leaves nothing to blend, so every color has to
// come out of the palette or the terminal's own default.
func TestTerminalThemeFallsBackToThePalette(t *testing.T) {
	theme := buildTerminalTheme(terminalSurface{})

	for name, color := range map[string]tcell.Color{
		"Background": theme.Background, "Foreground": theme.Foreground,
		"Border": theme.Border, "BorderFocus": theme.BorderFocus,
		"SelectionText": theme.SelectionText, "SelectionBg": theme.SelectionBg,
		"HeaderBg": theme.HeaderBg, "HeaderText": theme.HeaderText,
		"SecondaryText": theme.SecondaryText, "Accent": theme.Accent,
		"InputBg": theme.InputBg, "InverseText": theme.InverseText,
		"AssigneeText": theme.AssigneeText, "Success": theme.Success,
		"StatusTriage": theme.StatusTriage, "StatusTodo": theme.StatusTodo,
		"StatusInProgress": theme.StatusInProgress, "StatusReview": theme.StatusReview,
		"StatusDone": theme.StatusDone, "StatusCanceled": theme.StatusCanceled,
	} {
		if color.IsRGB() {
			t.Errorf("%s is %s, which pins a color the terminal has no say in", name, colorTag(color))
		}
	}
}

// A palette color has to reach tview and glamour as a slot, not as the hex of
// a standard palette the terminal has replaced.
func TestPaletteColorsSurviveTheConversions(t *testing.T) {
	if got := colorName(ansiBlue); got != "navy" {
		t.Errorf("colorName(ANSI 4) = %q, want the palette name", got)
	}
	if got := tcell.GetColor(colorName(ansiBlue)); got != ansiBlue {
		t.Errorf("tview would resolve the tag back to %v, want ANSI 4", got)
	}

	got := hexPtr(ansiBlue)
	if got == nil || *got != "4" {
		t.Errorf("hexPtr(ANSI 4) = %v, want the palette index", got)
	}
	if rgb := hexPtr(tcell.NewRGBColor(1, 2, 3)); rgb == nil || *rgb != "#010203" {
		t.Errorf("hexPtr of an RGB color = %v, want the hex", rgb)
	}
}
