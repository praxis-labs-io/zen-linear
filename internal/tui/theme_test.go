package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
)

// TestResolveThemeKnownNames verifies every registered theme resolves by name.
func TestResolveThemeKnownNames(t *testing.T) {
	tests := []struct {
		name     string
		expected Theme
	}{
		{config.ThemeLinear, LinearTheme},
		{config.ThemeHighContrast, HighContrastTheme},
		{config.ThemeColorBlind, ColorBlindTheme},
		{config.ThemeRosePineMoon, RosePineMoonTheme},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveTheme(tt.name); got != tt.expected {
				t.Errorf("ResolveTheme(%q) returned unexpected theme", tt.name)
			}
		})
	}
}

// An unknown name returns the adaptive theme, so a config naming a theme that
// has gone away still follows the terminal.
func TestResolveThemeUnknownFallsBack(t *testing.T) {
	if got := ResolveTheme("rainbow"); got != TerminalTheme() {
		t.Errorf("ResolveTheme(\"rainbow\") = %+v, want the terminal theme", got)
	}
}

// TestInverseTextColor verifies the explicit inverse color wins and legacy
// themes fall back to their background.
func TestInverseTextColor(t *testing.T) {
	if got := LinearTheme.InverseTextColor(); got != LinearTheme.Background {
		t.Errorf("LinearTheme.InverseTextColor() = %v, want Background %v", got, LinearTheme.Background)
	}

	want := tcell.NewRGBColor(35, 33, 54)
	if got := RosePineMoonTheme.InverseTextColor(); got != want {
		t.Errorf("RosePineMoonTheme.InverseTextColor() = %v, want %v", got, want)
	}
}

// TestThemeTagsAssigneeUsesTheAccessor covers the tag a comment byline colors
// its author with. Built off the raw field, a theme that predates it would tag
// as [default] rather than falling back to the foreground.
func TestThemeTagsAssigneeUsesTheAccessor(t *testing.T) {
	if got := NewThemeTags(LinearTheme).AssigneeText; got != colorTag(LinearTheme.AssigneeText) {
		t.Errorf("AssigneeText tag = %q, want the theme's own color %q", got, colorTag(LinearTheme.AssigneeText))
	}

	legacy := LinearTheme
	legacy.AssigneeText = tcell.ColorDefault
	if got := NewThemeTags(legacy).AssigneeText; got != colorTag(legacy.Foreground) {
		t.Errorf("AssigneeText tag with the field unset = %q, want the foreground %q", got, colorTag(legacy.Foreground))
	}
}

// TestRosePineMoonBackgroundTransparent pins the transparent background: the
// theme must keep tcell.ColorDefault so the terminal background shows through.
func TestRosePineMoonBackgroundTransparent(t *testing.T) {
	if RosePineMoonTheme.Background != tcell.ColorDefault {
		t.Errorf("RosePineMoonTheme.Background = %v, want tcell.ColorDefault", RosePineMoonTheme.Background)
	}
}

// TestRosePineMoonStaysInItsPalette is the guard on the borrowed color. The
// palette has six hues and no green, and a role whose convention is green used
// to carry a hex from another theme: on screen that read as a color the rest of
// the app never uses. Every field here has to come from the palette above.
func TestRosePineMoonStaysInItsPalette(t *testing.T) {
	palette := map[tcell.Color]bool{
		tcell.ColorDefault:    true,
		rosePineBase:          true,
		rosePineSurface:       true,
		rosePineOverlay:       true,
		rosePineMuted:         true,
		rosePineSubtle:        true,
		rosePineText:          true,
		rosePineLove:          true,
		rosePineGold:          true,
		rosePineRose:          true,
		rosePinePine:          true,
		rosePineFoam:          true,
		rosePineIris:          true,
		rosePineHighlightMed:  true,
		rosePineHighlightHigh: true,
	}

	theme := RosePineMoonTheme
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
		if !palette[color] {
			t.Errorf("RosePineMoonTheme.%s is %s, which is not in the Rosé Pine Moon palette", name, colorTag(color))
		}
	}
}
