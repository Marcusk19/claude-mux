package ui

import "github.com/charmbracelet/lipgloss"

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 1)

	workingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	waitingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	permissionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	doneStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))

	pathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	branchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	agoStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	activityStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Italic(true)
	footerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
)
