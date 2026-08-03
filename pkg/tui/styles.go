package tui

import "github.com/charmbracelet/lipgloss"

var (
	fgColor        = lipgloss.Color("#C0C0C0")
	highlightColor = lipgloss.Color("#FFFFFF")
	mutedColor     = lipgloss.Color("#666666")
	borderColor    = lipgloss.Color("#444444")

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlightColor).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(fgColor).
			Bold(true)

	HeaderBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(1, 2).
			MarginBottom(1)

	ItemStyle = lipgloss.NewStyle().
			Foreground(fgColor).
			PaddingLeft(2)

	SelectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(highlightColor).
				Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	HelpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#55FF55")).
			Bold(true)
)
