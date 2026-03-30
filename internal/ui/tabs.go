package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Tab represents which tab is active.
type Tab int

const (
	TabGlobal    Tab = iota
	TabKanban
	TabWorktrees
	TabCC
)

var tabNames = []string{"Global", "Local", "Worktrees", "CC"}

var (
	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 2)

	tabBarStyle = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottomForeground(lipgloss.Color("238"))
)

func renderTabBar(active Tab, width int) string {
	var tabs []string
	for i, name := range tabNames {
		if Tab(i) == active {
			tabs = append(tabs, activeTabStyle.Render(name))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(name))
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return tabBarStyle.Width(width).Render(row) + "\n"
}

func tabBarHeight() int {
	// tab row + border bottom + newline
	return strings.Count(renderTabBar(TabGlobal, 80), "\n") + 1
}
