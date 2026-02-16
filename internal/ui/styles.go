package ui

import "github.com/charmbracelet/lipgloss"

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 1)

	workingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	waitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	pathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	branchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	agoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)
