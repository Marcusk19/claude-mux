package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mkok/claude-mux/internal/cc"
)

func renderCCView(state *cc.State, width, height int) string {
	if state == nil {
		msg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(2, 4).
			Render("Command Center is not running. Press Enter to start.")
		return msg
	}

	uptime := time.Since(state.CreatedAt).Truncate(time.Second)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(width - 4).
		Render(fmt.Sprintf(
			"%s  %s\n%s  %s\n%s  %s\n\n%s",
			lipgloss.NewStyle().Bold(true).Render("Pane:"),
			state.PaneID,
			lipgloss.NewStyle().Bold(true).Render("Uptime:"),
			uptime,
			lipgloss.NewStyle().Bold(true).Render("Repo:"),
			state.RepoRoot,
			lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Press Enter to open Command Center"),
		))

	return card
}
