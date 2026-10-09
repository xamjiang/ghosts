package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// theme follows Apple's semantic colors: a label hierarchy of three greys,
// one accent, and status colors used only where they carry meaning. Light
// variants are darkened for legible text on white.
type theme struct {
	fg, sub, faint, line  color.Color
	selBG, pillBG         color.Color
	accent, green, orange color.Color
	red                   color.Color
}

func newTheme(dark bool) theme {
	if dark {
		return theme{
			fg:     lipgloss.Color("#F5F5F7"),
			sub:    lipgloss.Color("#98989D"),
			faint:  lipgloss.Color("#636366"),
			line:   lipgloss.Color("#3A3A3C"),
			selBG:  lipgloss.Color("#2C2C2E"),
			pillBG: lipgloss.Color("#3A3A3C"),
			accent: lipgloss.Color("#0A84FF"),
			green:  lipgloss.Color("#30D158"),
			orange: lipgloss.Color("#FF9F0A"),
			red:    lipgloss.Color("#FF453A"),
		}
	}
	return theme{
		fg:     lipgloss.Color("#1D1D1F"),
		sub:    lipgloss.Color("#6E6E73"),
		faint:  lipgloss.Color("#A1A1A6"),
		line:   lipgloss.Color("#D2D2D7"),
		selBG:  lipgloss.Color("#E8E8ED"),
		pillBG: lipgloss.Color("#E3E3E8"),
		accent: lipgloss.Color("#0066CC"),
		green:  lipgloss.Color("#248A3D"),
		orange: lipgloss.Color("#B25000"),
		red:    lipgloss.Color("#D70015"),
	}
}

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
