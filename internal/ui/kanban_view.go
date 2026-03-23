package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mkok/claude-mux/internal/kanban"
	"github.com/mkok/claude-mux/internal/session"
)

// kanbanMsg carries discovered kanban cards to the TUI.
type kanbanMsg []kanban.PaneCard

func stateEmoji(s session.ActivityState) string {
	switch s {
	case session.StateWorking:
		return "\u23f3"
	case session.StateWaiting:
		return "\U0001f4ac"
	case session.StatePermission:
		return "\U0001f510"
	case session.StateDone:
		return "\u2705"
	default:
		return "\u2753"
	}
}

func stateBorderColor(s session.ActivityState) lipgloss.Color {
	switch s {
	case session.StateWorking:
		return lipgloss.Color("40")
	case session.StateWaiting, session.StatePermission:
		return lipgloss.Color("214")
	case session.StateDone:
		return lipgloss.Color("245")
	default:
		return lipgloss.Color("245")
	}
}

func kanbanWordWrap(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > width {
				lines = append(lines, line)
				line = w
			} else {
				line += " " + w
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func renderKanban(cards []kanban.PaneCard, selectedCard int, width, height int) string {
	if len(cards) == 0 {
		msg := "No Claude agents in this window"
		pad := ""
		if width > len(msg) {
			pad = strings.Repeat(" ", (width-len(msg))/2)
		}
		vpad := ""
		if height > 1 {
			vpad = strings.Repeat("\n", height/2)
		}
		return vpad + pad + msg
	}

	gap := 1
	totalGaps := (len(cards) - 1) * gap
	colWidth := (width - totalGaps) / len(cards)
	if colWidth < 20 {
		colWidth = 20
	}

	contentWidth := colWidth - 4 // account for border + padding

	selectedBorderColor := lipgloss.Color("117")

	var columns []string
	for i, card := range cards {
		borderColor := stateBorderColor(card.State)
		if i == selectedCard {
			borderColor = selectedBorderColor
		}

		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(colWidth - 2). // subtract border width
			Padding(0, 1)

		// Line 1: state emoji + Pane index
		line1 := lipgloss.NewStyle().Bold(true).
			Render(fmt.Sprintf("%s Pane %s", stateEmoji(card.State), card.Pane.PaneIndex))

		// Line 2: branch name
		line2 := ""
		if card.GitBranch != "" {
			line2 = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).
				Render(card.GitBranch)
		}

		// Line 3: tool or status
		line3 := ""
		if card.LiveTool != "" {
			line3 = card.LiveTool
		} else {
			line3 = card.State.String()
		}

		// Line 4+: LiveStatus or Summary, word-wrapped
		text := card.LiveStatus
		if text == "" {
			text = card.Summary
		}

		var content []string
		content = append(content, line1)
		if line2 != "" {
			content = append(content, line2)
		}
		content = append(content, line3)

		if text != "" {
			wrapped := kanbanWordWrap(text, contentWidth)
			// Truncate to fit available height (subtract header lines + borders)
			maxLines := height - len(content) - 2 // 2 for top/bottom border
			if maxLines < 1 {
				maxLines = 1
			}
			if len(wrapped) > maxLines {
				wrapped = wrapped[:maxLines]
			}
			content = append(content, wrapped...)
		}

		columns = append(columns, cardStyle.Render(strings.Join(content, "\n")))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, columns...)
}

func pollKanbanCmd(sessionName, windowIndex string) tea.Cmd {
	return func() tea.Msg {
		cards, _ := kanban.DiscoverKanban(sessionName, windowIndex)
		return kanbanMsg(cards)
	}
}
