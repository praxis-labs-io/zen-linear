package tui

import "github.com/gdamore/tcell/v2"

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

	// Status Colors
	StatusTodo       tcell.Color
	StatusInProgress tcell.Color
	StatusDone       tcell.Color
	StatusCanceled   tcell.Color
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

	StatusTodo:       tcell.NewRGBColor(140, 140, 140), // Gray
	StatusInProgress: tcell.NewRGBColor(242, 201, 76),  // Yellow
	StatusDone:       tcell.NewRGBColor(94, 106, 210),  // Purple/Blue (Linear uses purple for done often, or green)
	StatusCanceled:   tcell.NewRGBColor(255, 80, 80),   // Red
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
