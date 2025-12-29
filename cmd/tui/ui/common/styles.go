package common

import "github.com/charmbracelet/lipgloss"

var (
	Primary = lipgloss.Color("62")
	Success = lipgloss.Color("42")
	Error   = lipgloss.Color("196")
	Muted   = lipgloss.Color("245")

	PanelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2)

	InputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Muted).
			Padding(0, 1)

	InputFocusedStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(Primary).
				Padding(0, 1)

	InputTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	InputPlaceholderStyle = lipgloss.NewStyle().
				Foreground(Muted)

	ErrorTextStyle = lipgloss.NewStyle().
			Foreground(Error)

	SuccessTextStyle = lipgloss.NewStyle().
			Foreground(Success)

	MutedTextStyle = lipgloss.NewStyle().
			Foreground(Muted)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)
)
