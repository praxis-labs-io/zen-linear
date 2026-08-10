package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/config"
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

// TestResolveThemeUnknownFallsBack verifies unknown names return the default theme.
func TestResolveThemeUnknownFallsBack(t *testing.T) {
	if got := ResolveTheme("rainbow"); got != LinearTheme {
		t.Errorf("ResolveTheme(\"rainbow\") = %+v, want LinearTheme", got)
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
