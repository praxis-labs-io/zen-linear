package tui

import "github.com/roeyazroel/linear-tui/internal/config"

// DensityProfile defines spacing values for UI layouts.
type DensityProfile struct {
	ID                 string
	DetailsPadding     Padding
	ModalPadding       Padding
	StatusBarPadding   Padding
	PaletteSpacerLines int
	DetailsSectionGap  int
}

// Padding captures per-side padding values.
type Padding struct {
	Top    int
	Bottom int
	Left   int
	Right  int
}

// DensityRegistry maps density identifiers to profiles.
var DensityRegistry = map[string]DensityProfile{
	config.DensityComfortable: {
		ID: config.DensityComfortable,
		DetailsPadding: Padding{
			Top: 1, Bottom: 1, Left: 2, Right: 2,
		},
		ModalPadding: Padding{
			Top: 1, Bottom: 1, Left: 2, Right: 2,
		},
		StatusBarPadding: Padding{
			Top: 0, Bottom: 0, Left: 1, Right: 1,
		},
		PaletteSpacerLines: 1,
		DetailsSectionGap:  1,
	},
	config.DensityCompact: {
		ID: config.DensityCompact,
		DetailsPadding: Padding{
			Top: 0, Bottom: 0, Left: 1, Right: 1,
		},
		ModalPadding: Padding{
			Top: 0, Bottom: 0, Left: 1, Right: 1,
		},
		StatusBarPadding: Padding{
			Top: 0, Bottom: 0, Left: 0, Right: 1,
		},
		PaletteSpacerLines: 0,
		DetailsSectionGap:  0,
	},
}

// ResolveDensity returns the density profile for a given name, or the default.
func ResolveDensity(name string) DensityProfile {
	if density, ok := DensityRegistry[name]; ok {
		return density
	}
	return DensityRegistry[config.DefaultDensity]
}
