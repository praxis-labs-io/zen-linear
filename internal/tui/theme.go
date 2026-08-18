package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
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

	// AssigneeText colors the assignee initials in the issue list. Zero value
	// falls back to Foreground.
	AssigneeText tcell.Color

	// Success is what a finished action is said in. Separate from the status
	// palette on purpose: StatusReview happens to be green in every theme
	// shipped today, and borrowing it would recolor every success toast the
	// day one of them makes review orange. Zero value falls back to it.
	Success tcell.Color

	// Status Colors
	StatusTriage     tcell.Color // zero value falls back to StatusTodo
	StatusTodo       tcell.Color
	StatusInProgress tcell.Color
	StatusReview     tcell.Color // zero value falls back to StatusDone
	StatusDone       tcell.Color
	StatusCanceled   tcell.Color
}

// ModalBackground returns the panel color for modals and overlays: themes
// with a transparent background stay transparent, opaque themes keep the
// header background for contrast against the app surface.
func (t Theme) ModalBackground() tcell.Color {
	if t.Background == tcell.ColorDefault {
		return tcell.ColorDefault
	}
	return t.HeaderBg
}

// InverseTextColor returns the color for inverse-video text, falling back to
// the theme background when no explicit inverse color is set.
func (t Theme) InverseTextColor() tcell.Color {
	if t.InverseText != tcell.ColorDefault {
		return t.InverseText
	}
	return t.Background
}

// AssigneeTextColor returns the color for assignee initials, falling back to
// the foreground for themes that predate the field.
func (t Theme) AssigneeTextColor() tcell.Color {
	if t.AssigneeText != tcell.ColorDefault {
		return t.AssigneeText
	}
	return t.Foreground
}

// StatusTriageColor returns the color for triage states, falling back to the
// todo color for themes that predate the field.
func (t Theme) StatusTriageColor() tcell.Color {
	if t.StatusTriage != tcell.ColorDefault {
		return t.StatusTriage
	}
	return t.StatusTodo
}

// StatusReviewColor returns the color for review states, falling back to the
// done color for themes that predate the field.
func (t Theme) StatusReviewColor() tcell.Color {
	if t.StatusReview != tcell.ColorDefault {
		return t.StatusReview
	}
	return t.StatusDone
}

// SuccessColor returns the color a finished action is said in, falling back to
// the review color for themes that predate the field.
func (t Theme) SuccessColor() tcell.Color {
	if t.Success != tcell.ColorDefault {
		return t.Success
	}
	return t.StatusReviewColor()
}

// LinearTheme is the default dark theme inspired by Linear.
var LinearTheme = Theme{
	Background:    tcell.NewRGBColor(18, 18, 18),    // #121212
	Foreground:    tcell.NewRGBColor(235, 235, 245), // #EBEBF5
	Border:        tcell.NewRGBColor(60, 60, 60),    // #3C3C3C
	BorderFocus:   tcell.NewRGBColor(94, 106, 210),  // #5E6AD2 (Linear Purple-ish)
	SelectionText: tcell.NewRGBColor(255, 255, 255), // #FFFFFF
	SelectionBg:   tcell.NewRGBColor(40, 40, 50),    // Slight purple tint dark bg
	HeaderBg:      tcell.NewRGBColor(30, 30, 30),    // #1E1E1E
	HeaderText:    tcell.NewRGBColor(160, 160, 160), // #A0A0A0
	SecondaryText: tcell.NewRGBColor(120, 120, 120), // #787878
	Accent:        tcell.NewRGBColor(94, 106, 210),  // #5E6AD2
	InputBg:       tcell.ColorDarkGray,
	AssigneeText:  tcell.NewRGBColor(242, 153, 74), // #F2994A orange

	Success: tcell.NewRGBColor(76, 183, 130), // #4CB782 green

	StatusTriage:     tcell.NewRGBColor(242, 153, 74),  // #F2994A orange
	StatusTodo:       tcell.NewRGBColor(140, 140, 140), // Gray
	StatusInProgress: tcell.NewRGBColor(242, 201, 76),  // Yellow
	StatusReview:     tcell.NewRGBColor(76, 183, 130),  // #4CB782 green
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
	AssigneeText:  tcell.NewRGBColor(255, 128, 0),   // #FF8000 orange

	Success: tcell.NewRGBColor(0, 255, 0), // #00FF00 green

	StatusTriage:     tcell.NewRGBColor(255, 128, 0),   // #FF8000 orange
	StatusTodo:       tcell.NewRGBColor(255, 255, 255), // White
	StatusInProgress: tcell.NewRGBColor(255, 255, 0),   // Yellow
	StatusReview:     tcell.NewRGBColor(0, 255, 0),     // Green
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
	AssigneeText:  tcell.NewRGBColor(230, 159, 0),   // #E69F00 orange

	Success: tcell.NewRGBColor(0, 158, 115), // #009E73 bluish green

	StatusTriage:     tcell.NewRGBColor(230, 159, 0),   // #E69F00 orange
	StatusTodo:       tcell.NewRGBColor(153, 153, 153), // Gray
	StatusInProgress: tcell.NewRGBColor(86, 180, 233),  // #56B4E9
	StatusReview:     tcell.NewRGBColor(0, 158, 115),   // #009E73
	StatusDone:       tcell.NewRGBColor(0, 158, 115),   // #009E73
	StatusCanceled:   tcell.NewRGBColor(213, 94, 0),    // #D55E00
}

// The Rosé Pine Moon palette (rosepinetheme.com), named so the theme below
// reads as the palette rather than as a column of hexes. Every color the theme
// uses comes from here: the palette has six hues and no green, so a role with
// nothing to map to takes one of these rather than borrowing a color from
// outside it.
var (
	rosePineBase          = tcell.NewRGBColor(35, 33, 54)    // #232136
	rosePineSurface       = tcell.NewRGBColor(42, 39, 63)    // #2A273F
	rosePineOverlay       = tcell.NewRGBColor(57, 53, 82)    // #393552
	rosePineMuted         = tcell.NewRGBColor(110, 106, 134) // #6E6A86
	rosePineSubtle        = tcell.NewRGBColor(144, 140, 170) // #908CAA
	rosePineText          = tcell.NewRGBColor(224, 222, 244) // #E0DEF4
	rosePineLove          = tcell.NewRGBColor(235, 111, 146) // #EB6F92
	rosePineGold          = tcell.NewRGBColor(246, 193, 119) // #F6C177
	rosePineRose          = tcell.NewRGBColor(234, 154, 151) // #EA9A97
	rosePinePine          = tcell.NewRGBColor(62, 143, 176)  // #3E8FB0
	rosePineFoam          = tcell.NewRGBColor(156, 207, 216) // #9CCFD8
	rosePineIris          = tcell.NewRGBColor(196, 167, 231) // #C4A7E7
	rosePineHighlightMed  = tcell.NewRGBColor(68, 65, 90)    // #44415A
	rosePineHighlightHigh = tcell.NewRGBColor(86, 82, 110)   // #56526E
)

// RosePineMoonTheme is the Rosé Pine Moon palette with a transparent
// background: tcell.ColorDefault leaves the terminal's own background (and any
// transparency/blur) visible.
var RosePineMoonTheme = Theme{
	Background:    tcell.ColorDefault, // terminal default (transparent)
	Foreground:    rosePineText,
	Border:        rosePineHighlightHigh,
	BorderFocus:   rosePineIris,
	SelectionText: rosePineText,
	SelectionBg:   rosePineHighlightMed,
	HeaderBg:      rosePineSurface,
	HeaderText:    rosePineSubtle,
	SecondaryText: rosePineMuted,
	Accent:        rosePineIris,
	InputBg:       rosePineOverlay,
	InverseText:   rosePineBase,
	AssigneeText:  rosePineRose,

	// Green is the convention for both of these and Rosé Pine has none. Iris is
	// what is left once the other four hues are spoken for below, and it is
	// already the palette's own emphasis color.
	Success:      rosePineIris,
	StatusReview: rosePineIris,

	StatusTriage:     rosePineRose,
	StatusTodo:       rosePinePine,
	StatusInProgress: rosePineGold,
	StatusDone:       rosePineFoam,
	StatusCanceled:   rosePineLove,
}

// ThemeTags provides tview tag strings derived from a theme.
type ThemeTags struct {
	Foreground    string
	SecondaryText string
	HeaderText    string
	Accent        string
	AssigneeText  string
	Border        string
	BorderFocus   string
	Warning       string
	Success       string
	Error         string
	// Selection is the cursor line, carrying a background as well as a color.
	// It is what selectionStyle paints for the tree and the issue tables.
	Selection string
}

// ThemeRegistry maps theme identifiers to theme palettes.
var ThemeRegistry = map[string]Theme{
	config.ThemeLinear:       LinearTheme,
	config.ThemeHighContrast: HighContrastTheme,
	config.ThemeColorBlind:   ColorBlindTheme,
	config.ThemeRosePineMoon: RosePineMoonTheme,
}

// ResolveTheme returns the theme for a name, or the terminal-derived one. That
// one is built rather than registered: its shades come from a launch query.
func ResolveTheme(name string) Theme {
	if theme, ok := ThemeRegistry[name]; ok {
		return theme
	}
	return TerminalTheme()
}

// NewThemeTags builds tag strings for dynamic color usage.
func NewThemeTags(theme Theme) ThemeTags {
	return ThemeTags{
		Foreground:    colorTag(theme.Foreground),
		SecondaryText: colorTag(theme.SecondaryText),
		HeaderText:    colorTag(theme.HeaderText),
		Accent:        colorTag(theme.Accent),
		// Through the accessor, not the field: an unset optional color is
		// ColorDefault, which colorTag would hand back as [default].
		AssigneeText: colorTag(theme.AssigneeTextColor()),
		Border:       colorTag(theme.Border),
		BorderFocus:  colorTag(theme.BorderFocus),
		Warning:      colorTag(theme.StatusInProgress),
		Success:      colorTag(theme.SuccessColor()),
		Error:        colorTag(theme.StatusCanceled),
		Selection:    fmt.Sprintf("[%s:%s]", colorName(theme.SelectionText), colorName(theme.SelectionBg)),
	}
}

func colorTag(color tcell.Color) string {
	return "[" + colorName(color) + "]"
}

// paletteNames are the terminal's own slots as tview spells them. A table
// rather than tcell.Name(), whose map walk answers with a random alias.
var paletteNames = [16]string{
	"black", "maroon", "green", "olive", "navy", "purple", "teal", "silver",
	"gray", "red", "lime", "yellow", "blue", "fuchsia", "aqua", "white",
}

// colorName is a color as tview spells it inside a tag. A palette color is
// named, not hexed: a hex would pin it to a palette the terminal has replaced.
func colorName(color tcell.Color) string {
	if !color.Valid() {
		return "default"
	}
	if slot := int(color &^ tcell.ColorValid); !color.IsRGB() && slot < len(paletteNames) {
		return paletteNames[slot]
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
	return css
}

// Icons for various UI elements.
var Icons = struct {
	Team       string
	Project    string
	List       string
	Todo       string
	InProgress string
	Done       string
	Canceled   string
	Priority   string
}{
	Team:       "📁 ",
	Project:    "📄 ",
	List:       "📑 ",
	Todo:       "○ ",
	InProgress: "◐ ",
	Done:       "✔ ", // or ●
	Canceled:   "✕ ",
	Priority:   "⚡",
}
