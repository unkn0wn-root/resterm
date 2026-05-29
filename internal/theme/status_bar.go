package theme

import "github.com/charmbracelet/lipgloss"

type StatusBarSegmentStyle struct {
	Foreground lipgloss.Color
	Background lipgloss.Color
}

type StatusBarPalette struct {
	Base      lipgloss.Color
	Info      StatusBarSegmentStyle
	Warn      StatusBarSegmentStyle
	Error     StatusBarSegmentStyle
	Success   StatusBarSegmentStyle
	File      StatusBarSegmentStyle
	Focus     StatusBarSegmentStyle
	Mode      StatusBarSegmentStyle
	Zoom      StatusBarSegmentStyle
	Minimized StatusBarSegmentStyle
	Version   StatusBarSegmentStyle
	User      StatusBarSegmentStyle
	Host      StatusBarSegmentStyle
}

func DefaultStatusBarPalette() StatusBarPalette {
	return StatusBarPalette{
		Base:      lipgloss.Color("#000000"),
		Info:      StatusBarSegmentStyle{Foreground: "#EFF6FF", Background: "#2563EB"},
		Warn:      StatusBarSegmentStyle{Foreground: "#FFF7ED", Background: "#B45309"},
		Error:     StatusBarSegmentStyle{Foreground: "#FEF2F2", Background: "#B91C1C"},
		Success:   StatusBarSegmentStyle{Foreground: "#F0FDF4", Background: "#15803D"},
		File:      StatusBarSegmentStyle{Foreground: "#F9FAFB", Background: "#404040"},
		Focus:     StatusBarSegmentStyle{Foreground: "#F4F4F5", Background: "#52525B"},
		Mode:      StatusBarSegmentStyle{Foreground: "#F8FAFC", Background: "#64748B"},
		Zoom:      StatusBarSegmentStyle{Foreground: "#ECFEFF", Background: "#0891B2"},
		Minimized: StatusBarSegmentStyle{Foreground: "#F0FDF4", Background: "#166534"},
		Version:   StatusBarSegmentStyle{Foreground: "#F8FAFC", Background: "#374151"},
		User:      StatusBarSegmentStyle{Foreground: "#F8FAFC", Background: "#4B5563"},
		Host:      StatusBarSegmentStyle{Foreground: "#E5E7EB", Background: "#27272A"},
	}
}
