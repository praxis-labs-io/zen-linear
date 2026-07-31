package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/roeyazroel/linear-tui/internal/config"
)

// Theme defines the color palette and styles for the application.
type Theme struct {
	Background    tcell.Color
	Foreground    tcell.Color
	Border        tcell.Color
	BorderFocus   tcell.Color
	SelectionText tcell.Color
	SelectionBg   tcell.Color
	HeaderBg      tcell.Color
	HeaderText    tcell.Color
	SecondaryText tcell.Color
	Accent        tcell.Color
	InputBg       tcell.Color

	// InverseText overrides the color used for inverse-video text. Themes
	// with a transparent Background must set it, since Background is not a
	// paintable color there. Zero value falls back to Background.
	InverseText tcell.Color

	// Status Colors
	StatusTodo       tcell.Color
	StatusInProgress tcell.Color
	StatusDone       tcell.Color
	StatusCanceled   tcell.Color
}

// InverseTextColor returns the color for inverse-video text, falling back to
// the theme background when no explicit inverse color is set.
func (t Theme) InverseTextColor() tcell.Color {
	if t.InverseText != tcell.ColorDefault {
		return t.InverseText
	}
	return t.Background
}

// LinearTheme is the default dark theme inspired by Linear.
var LinearTheme = Theme{
	Background:    tcell.NewRGBColor(18, 18, 18),    // #121212
	Foreground:    tcell.NewRGBColor(235, 235, 245), // #EBEBF5
	Border:        tcell.NewRGBColor(60, 60, 60),    // #3C3C3C
	BorderFocus:   tcell.NewRGBColor(94, 106, 210),  // #5E6AD2 (Linear Purple-ish)
	SelectionText: tcell.ColorWhite,
	SelectionBg:   tcell.NewRGBColor(40, 40, 50),    // Slight purple tint dark bg
	HeaderBg:      tcell.NewRGBColor(30, 30, 30),    // #1E1E1E
	HeaderText:    tcell.NewRGBColor(160, 160, 160), // #A0A0A0
	SecondaryText: tcell.NewRGBColor(120, 120, 120), // #787878
	Accent:        tcell.NewRGBColor(94, 106, 210),  // #5E6AD2
	InputBg:       tcell.ColorDarkGray,

	StatusTodo:       tcell.NewRGBColor(140, 140, 140), // Gray
	StatusInProgress: tcell.NewRGBColor(242, 201, 76),  // Yellow
	StatusDone:       tcell.NewRGBColor(94, 106, 210),  // Purple/Blue (Linear uses purple for done often, or green)
	StatusCanceled:   tcell.NewRGBColor(255, 80, 80),   // Red
}

// HighContrastTheme is a high contrast theme for improved legibility.
var HighContrastTheme = Theme{
	Background:    tcell.NewRGBColor(0, 0, 0),       // #000000
	Foreground:    tcell.NewRGBColor(255, 255, 255), // #FFFFFF
	Border:        tcell.NewRGBColor(255, 255, 255), // #FFFFFF
	BorderFocus:   tcell.NewRGBColor(255, 255, 0),   // #FFFF00
	SelectionText: tcell.NewRGBColor(0, 0, 0),       // #000000
	SelectionBg:   tcell.NewRGBColor(255, 255, 255), // #FFFFFF
	HeaderBg:      tcell.NewRGBColor(0, 0, 0),       // #000000
	HeaderText:    tcell.NewRGBColor(255, 255, 255), // #FFFFFF
	SecondaryText: tcell.NewRGBColor(200, 200, 200), // #C8C8C8
	Accent:        tcell.NewRGBColor(255, 255, 0),   // #FFFF00
	InputBg:       tcell.NewRGBColor(30, 30, 30),    // #1E1E1E

	StatusTodo:       tcell.NewRGBColor(255, 255, 255), // White
	StatusInProgress: tcell.NewRGBColor(255, 255, 0),   // Yellow
	StatusDone:       tcell.NewRGBColor(0, 255, 0),     // Green
	StatusCanceled:   tcell.NewRGBColor(255, 0, 0),     // Red
}

// ColorBlindTheme is a color-blind friendly palette.
var ColorBlindTheme = Theme{
	Background:    tcell.NewRGBColor(16, 16, 16),    // #101010
	Foreground:    tcell.NewRGBColor(230, 230, 230), // #E6E6E6
	Border:        tcell.NewRGBColor(74, 74, 74),    // #4A4A4A
	BorderFocus:   tcell.NewRGBColor(0, 114, 178),   // #0072B2
	SelectionText: tcell.NewRGBColor(255, 255, 255), // #FFFFFF
	SelectionBg:   tcell.NewRGBColor(38, 54, 86),    // #263656
	HeaderBg:      tcell.NewRGBColor(28, 28, 28),    // #1C1C1C
	HeaderText:    tcell.NewRGBColor(207, 207, 207), // #CFCFCF
	SecondaryText: tcell.NewRGBColor(154, 154, 154), // #9A9A9A
	Accent:        tcell.NewRGBColor(0, 114, 178),   // #0072B2
	InputBg:       tcell.NewRGBColor(42, 42, 42),    // #2A2A2A

	StatusTodo:       tcell.NewRGBColor(153, 153, 153), // Gray
	StatusInProgress: tcell.NewRGBColor(86, 180, 233),  // #56B4E9
	StatusDone:       tcell.NewRGBColor(0, 158, 115),   // #009E73
	StatusCanceled:   tcell.NewRGBColor(213, 94, 0),    // #D55E00
}

// RosePineMoonTheme is the Rosé Pine Moon palette (rosepinetheme.com) with a
// transparent background: tcell.ColorDefault leaves the terminal's own
// background (and any transparency/blur) visible.
var RosePineMoonTheme = Theme{
	Background:    tcell.ColorDefault,               // terminal default (transparent)
	Foreground:    tcell.NewRGBColor(224, 222, 244), // #E0DEF4 text
	Border:        tcell.NewRGBColor(86, 82, 110),   // #56526E highlight high
	BorderFocus:   tcell.NewRGBColor(196, 167, 231), // #C4A7E7 iris
	SelectionText: tcell.NewRGBColor(224, 222, 244), // #E0DEF4 text
	SelectionBg:   tcell.NewRGBColor(68, 65, 90),    // #44415A highlight med
	HeaderBg:      tcell.NewRGBColor(42, 39, 63),    // #2A273F surface
	HeaderText:    tcell.NewRGBColor(144, 140, 170), // #908CAA subtle
	SecondaryText: tcell.NewRGBColor(110, 106, 134), // #6E6A86 muted
	Accent:        tcell.NewRGBColor(196, 167, 231), // #C4A7E7 iris
	InputBg:       tcell.NewRGBColor(57, 53, 82),    // #393552 overlay
	InverseText:   tcell.NewRGBColor(35, 33, 54),    // #232136 base

	StatusTodo:       tcell.NewRGBColor(110, 106, 134), // #6E6A86 muted
	StatusInProgress: tcell.NewRGBColor(246, 193, 119), // #F6C177 gold
	StatusDone:       tcell.NewRGBColor(156, 207, 216), // #9CCFD8 foam
	StatusCanceled:   tcell.NewRGBColor(235, 111, 146), // #EB6F92 love
}

// ThemeTags provides tview tag strings derived from a theme.
type ThemeTags struct {
	Foreground    string
	SecondaryText string
	HeaderText    string
	Accent        string
	Border        string
	Warning       string
	Error         string
}

// ThemeRegistry maps theme identifiers to theme palettes.
var ThemeRegistry = map[string]Theme{
	config.ThemeLinear:       LinearTheme,
	config.ThemeHighContrast: HighContrastTheme,
	config.ThemeColorBlind:   ColorBlindTheme,
	config.ThemeRosePineMoon: RosePineMoonTheme,
}

// ResolveTheme returns the theme for a given name, or the default theme.
func ResolveTheme(name string) Theme {
	if theme, ok := ThemeRegistry[name]; ok {
		return theme
	}
	return LinearTheme
}

// NewThemeTags builds tag strings for dynamic color usage.
func NewThemeTags(theme Theme) ThemeTags {
	return ThemeTags{
		Foreground:    colorTag(theme.Foreground),
		SecondaryText: colorTag(theme.SecondaryText),
		HeaderText:    colorTag(theme.HeaderText),
		Accent:        colorTag(theme.Accent),
		Border:        colorTag(theme.Border),
		Warning:       colorTag(theme.StatusInProgress),
		Error:         colorTag(theme.StatusCanceled),
	}
}

func colorTag(color tcell.Color) string {
	if !color.Valid() {
		return "[default]"
	}
	css := color.CSS()
	if css == "" {
		if color.IsRGB() {
			css = fmt.Sprintf("#%06x", color.Hex())
		}
	}
	if css == "" {
		css = "default"
	}
	return "[" + css + "]"
}

// Icons for various UI elements.
var Icons = struct {
	Team       string
	Project    string
	List       string
	Todo       string
	InProgress string
	Done       string
	Priority   string
}{
	Team:       "📁 ",
	Project:    "📄 ",
	List:       "📑 ",
	Todo:       "○ ",
	InProgress: "◐ ",
	Done:       "✔ ", // or ●
	Priority:   "⚡",
}
