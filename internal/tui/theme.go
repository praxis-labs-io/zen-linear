package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
)

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

	// InverseText must be set by a theme with a transparent Background, which is not a paintable color.
	InverseText tcell.Color

	AssigneeText tcell.Color

	// Success is separate from StatusReview so a theme recoloring review does not recolor every success toast.
	Success tcell.Color

	StatusTriage     tcell.Color
	StatusTodo       tcell.Color
	StatusInProgress tcell.Color
	StatusReview     tcell.Color
	StatusDone       tcell.Color
	StatusCanceled   tcell.Color
}

// ModalBackground is transparent for a transparent theme and HeaderBg otherwise.
func (t Theme) ModalBackground() tcell.Color {
	if t.Background == tcell.ColorDefault {
		return tcell.ColorDefault
	}
	return t.HeaderBg
}

// InverseTextColor returns InverseText, or Background when unset.
func (t Theme) InverseTextColor() tcell.Color {
	if t.InverseText != tcell.ColorDefault {
		return t.InverseText
	}
	return t.Background
}

// AssigneeTextColor returns AssigneeText, or Foreground when unset.
func (t Theme) AssigneeTextColor() tcell.Color {
	if t.AssigneeText != tcell.ColorDefault {
		return t.AssigneeText
	}
	return t.Foreground
}

// StatusTriageColor returns StatusTriage, or StatusTodo when unset.
func (t Theme) StatusTriageColor() tcell.Color {
	if t.StatusTriage != tcell.ColorDefault {
		return t.StatusTriage
	}
	return t.StatusTodo
}

// StatusReviewColor returns StatusReview, or StatusDone when unset.
func (t Theme) StatusReviewColor() tcell.Color {
	if t.StatusReview != tcell.ColorDefault {
		return t.StatusReview
	}
	return t.StatusDone
}

// SuccessColor returns Success, or the review color when unset.
func (t Theme) SuccessColor() tcell.Color {
	if t.Success != tcell.ColorDefault {
		return t.Success
	}
	return t.StatusReviewColor()
}

var LinearTheme = Theme{
	Background:    tcell.NewRGBColor(18, 18, 18),
	Foreground:    tcell.NewRGBColor(235, 235, 245),
	Border:        tcell.NewRGBColor(60, 60, 60),
	BorderFocus:   tcell.NewRGBColor(94, 106, 210),
	SelectionText: tcell.NewRGBColor(255, 255, 255),
	SelectionBg:   tcell.NewRGBColor(40, 40, 50),
	HeaderBg:      tcell.NewRGBColor(30, 30, 30),
	HeaderText:    tcell.NewRGBColor(160, 160, 160),
	SecondaryText: tcell.NewRGBColor(120, 120, 120),
	Accent:        tcell.NewRGBColor(94, 106, 210),
	InputBg:       tcell.ColorDarkGray,
	AssigneeText:  tcell.NewRGBColor(242, 153, 74),

	Success: tcell.NewRGBColor(76, 183, 130),

	StatusTriage:     tcell.NewRGBColor(242, 153, 74),
	StatusTodo:       tcell.NewRGBColor(140, 140, 140),
	StatusInProgress: tcell.NewRGBColor(242, 201, 76),
	StatusReview:     tcell.NewRGBColor(76, 183, 130),
	StatusDone:       tcell.NewRGBColor(94, 106, 210),
	StatusCanceled:   tcell.NewRGBColor(255, 80, 80),
}

var HighContrastTheme = Theme{
	Background:    tcell.NewRGBColor(0, 0, 0),
	Foreground:    tcell.NewRGBColor(255, 255, 255),
	Border:        tcell.NewRGBColor(255, 255, 255),
	BorderFocus:   tcell.NewRGBColor(255, 255, 0),
	SelectionText: tcell.NewRGBColor(0, 0, 0),
	SelectionBg:   tcell.NewRGBColor(255, 255, 255),
	HeaderBg:      tcell.NewRGBColor(0, 0, 0),
	HeaderText:    tcell.NewRGBColor(255, 255, 255),
	SecondaryText: tcell.NewRGBColor(200, 200, 200),
	Accent:        tcell.NewRGBColor(255, 255, 0),
	InputBg:       tcell.NewRGBColor(30, 30, 30),
	AssigneeText:  tcell.NewRGBColor(255, 128, 0),

	Success: tcell.NewRGBColor(0, 255, 0),

	StatusTriage:     tcell.NewRGBColor(255, 128, 0),
	StatusTodo:       tcell.NewRGBColor(255, 255, 255),
	StatusInProgress: tcell.NewRGBColor(255, 255, 0),
	StatusReview:     tcell.NewRGBColor(0, 255, 0),
	StatusDone:       tcell.NewRGBColor(0, 255, 0),
	StatusCanceled:   tcell.NewRGBColor(255, 0, 0),
}

var ColorBlindTheme = Theme{
	Background:    tcell.NewRGBColor(16, 16, 16),
	Foreground:    tcell.NewRGBColor(230, 230, 230),
	Border:        tcell.NewRGBColor(74, 74, 74),
	BorderFocus:   tcell.NewRGBColor(0, 114, 178),
	SelectionText: tcell.NewRGBColor(255, 255, 255),
	SelectionBg:   tcell.NewRGBColor(38, 54, 86),
	HeaderBg:      tcell.NewRGBColor(28, 28, 28),
	HeaderText:    tcell.NewRGBColor(207, 207, 207),
	SecondaryText: tcell.NewRGBColor(154, 154, 154),
	Accent:        tcell.NewRGBColor(0, 114, 178),
	InputBg:       tcell.NewRGBColor(42, 42, 42),
	AssigneeText:  tcell.NewRGBColor(230, 159, 0),

	Success: tcell.NewRGBColor(0, 158, 115),

	StatusTriage:     tcell.NewRGBColor(230, 159, 0),
	StatusTodo:       tcell.NewRGBColor(153, 153, 153),
	StatusInProgress: tcell.NewRGBColor(86, 180, 233),
	StatusReview:     tcell.NewRGBColor(0, 158, 115),
	StatusDone:       tcell.NewRGBColor(0, 158, 115),
	StatusCanceled:   tcell.NewRGBColor(213, 94, 0),
}

var (
	rosePineBase          = tcell.NewRGBColor(35, 33, 54)
	rosePineSurface       = tcell.NewRGBColor(42, 39, 63)
	rosePineOverlay       = tcell.NewRGBColor(57, 53, 82)
	rosePineMuted         = tcell.NewRGBColor(110, 106, 134)
	rosePineSubtle        = tcell.NewRGBColor(144, 140, 170)
	rosePineText          = tcell.NewRGBColor(224, 222, 244)
	rosePineLove          = tcell.NewRGBColor(235, 111, 146)
	rosePineGold          = tcell.NewRGBColor(246, 193, 119)
	rosePineRose          = tcell.NewRGBColor(234, 154, 151)
	rosePinePine          = tcell.NewRGBColor(62, 143, 176)
	rosePineFoam          = tcell.NewRGBColor(156, 207, 216)
	rosePineIris          = tcell.NewRGBColor(196, 167, 231)
	rosePineHighlightMed  = tcell.NewRGBColor(68, 65, 90)
	rosePineHighlightHigh = tcell.NewRGBColor(86, 82, 110)
)

var RosePineMoonTheme = Theme{
	Background:    tcell.ColorDefault,
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

	Success:      rosePineIris,
	StatusReview: rosePineIris,

	StatusTriage:     rosePineRose,
	StatusTodo:       rosePinePine,
	StatusInProgress: rosePineGold,
	StatusDone:       rosePineFoam,
	StatusCanceled:   rosePineLove,
}

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
	Selection     string
}

var ThemeRegistry = map[string]Theme{
	config.ThemeLinear:       LinearTheme,
	config.ThemeHighContrast: HighContrastTheme,
	config.ThemeColorBlind:   ColorBlindTheme,
	config.ThemeRosePineMoon: RosePineMoonTheme,
}

// ResolveTheme returns the registered theme for name, or the terminal-derived theme.
func ResolveTheme(name string) Theme {
	if theme, ok := ThemeRegistry[name]; ok {
		return theme
	}
	return TerminalTheme()
}

func NewThemeTags(theme Theme) ThemeTags {
	return ThemeTags{
		Foreground:    colorTag(theme.Foreground),
		SecondaryText: colorTag(theme.SecondaryText),
		HeaderText:    colorTag(theme.HeaderText),
		Accent:        colorTag(theme.Accent),
		AssigneeText:  colorTag(theme.AssigneeTextColor()),
		Border:        colorTag(theme.Border),
		BorderFocus:   colorTag(theme.BorderFocus),
		Warning:       colorTag(theme.StatusInProgress),
		Success:       colorTag(theme.SuccessColor()),
		Error:         colorTag(theme.StatusCanceled),
		Selection:     fmt.Sprintf("[%s:%s]", colorName(theme.SelectionText), colorName(theme.SelectionBg)),
	}
}

func colorTag(color tcell.Color) string {
	return "[" + colorName(color) + "]"
}

// A table rather than tcell.Name(), whose map walk answers with a random alias.
var paletteNames = [16]string{
	"black", "maroon", "green", "olive", "navy", "purple", "teal", "silver",
	"gray", "red", "lime", "yellow", "blue", "fuchsia", "aqua", "white",
}

// A palette color is named, not hexed: a hex would pin it to a palette the terminal has replaced.
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
	Done:       "✔ ",
	Canceled:   "✕ ",
	Priority:   "⚡",
}
