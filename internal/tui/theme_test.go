package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
)

func TestResolveThemeKnownNames(t *testing.T) {
	tests := []struct {
		name     string
		expected Theme
	}{
		{config.ThemeLinear, LinearTheme},
		{config.ThemeHighContrast, HighContrastTheme},
		{config.ThemeColorBlind, ColorBlindTheme},
		{config.ThemeRosePineMoon, RosePineMoonTheme},
		{config.ThemeZenInk, ZenInkTheme},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveTheme(tt.name); got != tt.expected {
				t.Errorf("ResolveTheme(%q) returned unexpected theme", tt.name)
			}
		})
	}
}

func TestResolveThemeUnknownFallsBack(t *testing.T) {
	if got := ResolveTheme("rainbow"); got != TerminalTheme() {
		t.Errorf("ResolveTheme(\"rainbow\") = %+v, want the terminal theme", got)
	}
}

func TestInverseTextColor(t *testing.T) {
	if got := LinearTheme.InverseTextColor(); got != LinearTheme.Background {
		t.Errorf("LinearTheme.InverseTextColor() = %v, want Background %v", got, LinearTheme.Background)
	}

	want := tcell.NewRGBColor(35, 33, 54)
	if got := RosePineMoonTheme.InverseTextColor(); got != want {
		t.Errorf("RosePineMoonTheme.InverseTextColor() = %v, want %v", got, want)
	}
}

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

func TestRosePineMoonBackgroundTransparent(t *testing.T) {
	if RosePineMoonTheme.Background != tcell.ColorDefault {
		t.Errorf("RosePineMoonTheme.Background = %v, want tcell.ColorDefault", RosePineMoonTheme.Background)
	}
}

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

func TestZenInkPinsItsColorsToItsPalette(t *testing.T) {
	palette := map[tcell.Color]bool{
		zenInkBackground: true, zenInkSurface: true, zenInkOverlay: true, zenInkSelection: true,
		zenInkForeground: true, zenInkMuted: true, zenInkSubtle: true,
		zenInkCobalt: true, zenInkMint: true, zenInkAmber: true,
		zenInkCoral: true, zenInkViolet: true, zenInkCyan: true,
		zenInkBrightWhite: true,
	}

	theme := ZenInkTheme
	for name, color := range map[string]tcell.Color{
		"Background": theme.Background, "Foreground": theme.Foreground,
		"Border": theme.Border, "BorderFocus": theme.BorderFocus,
		"SelectionText": theme.SelectionText, "SelectionBg": theme.SelectionBg,
		"HeaderBg": theme.HeaderBg, "HeaderText": theme.HeaderText,
		"SecondaryText": theme.SecondaryText, "Accent": theme.Accent,
		"InputBg": theme.InputBg, "InverseText": theme.InverseTextColor(),
		"AssigneeText": theme.AssigneeTextColor(), "Success": theme.SuccessColor(),
		"StatusTriage": theme.StatusTriageColor(), "StatusTodo": theme.StatusTodo,
		"StatusInProgress": theme.StatusInProgress, "StatusReview": theme.StatusReviewColor(),
		"StatusDone": theme.StatusDone, "StatusCanceled": theme.StatusCanceled,
	} {
		if !palette[color] {
			t.Errorf("ZenInkTheme.%s is %s, which is not a pinned Zen Ink color", name, colorTag(color))
		}
	}
}

func TestFormControlsStandOffTheModalPanel(t *testing.T) {
	for name, theme := range ThemeRegistry {
		if theme.ModalBackground() == tcell.ColorDefault {
			continue
		}
		if theme.InputBg == theme.ModalBackground() {
			t.Errorf("%s: InputBg is the modal background %s, so unfocused buttons and checkboxes have no fill", name, colorTag(theme.InputBg))
		}
	}
}
