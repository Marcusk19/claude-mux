package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/Marcusk19/claude-mux/internal/kanban"
	"github.com/Marcusk19/claude-mux/internal/session"
)

// kanbanMsg carries discovered kanban cards to the TUI.
type kanbanMsg []kanban.PaneCard

// kanbanBoardMsg carries a loaded board to the TUI.
type kanbanBoardMsg struct {
	board *kanban.Board
}

// KanbanCard is the view-layer card used for rendering.
type KanbanCard struct {
	ID          string
	Title       string
	Description string
	Branch      string
	PaneID      string
	State       session.ActivityState
	LiveStatus  string
	LiveTool    string
}

// KanbanColumn is a named column containing cards.
type KanbanColumn struct {
	Name  string
	Cards []KanbanCard
}

// KanbanBoard holds the 3 columns for rendering.
type KanbanBoard struct {
	Columns [3]KanbanColumn // backlog, in-progress, done
}

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

// buildKanbanBoard constructs a KanbanBoard from a Board and live PaneCards.
// If board is nil, all pane cards go into the in-progress column.
func buildKanbanBoard(board *kanban.Board, paneCards []kanban.PaneCard) KanbanBoard {
	kb := KanbanBoard{
		Columns: [3]KanbanColumn{
			{Name: "Backlog"},
			{Name: "In-Progress"},
			{Name: "Done"},
		},
	}

	if board == nil {
		// No board file — all pane cards become in-progress cards
		for _, pc := range paneCards {
			kb.Columns[1].Cards = append(kb.Columns[1].Cards, KanbanCard{
				Title:      fmt.Sprintf("Pane %s", pc.Pane.PaneIndex),
				Branch:     pc.GitBranch,
				PaneID:     pc.Pane.PaneID,
				State:      pc.State,
				LiveStatus: pc.LiveStatus,
				LiveTool:   pc.LiveTool,
			})
		}
		return kb
	}

	// Build a lookup of pane cards by pane ID for enrichment
	paneByID := make(map[string]kanban.PaneCard)
	for _, pc := range paneCards {
		if pc.Pane.PaneID != "" {
			paneByID[pc.Pane.PaneID] = pc
		}
	}

	// Map board columns to the 3-column layout
	colMapping := map[string]int{
		"backlog":     0,
		"in-progress": 1,
		"done":        2,
	}

	for colName, colIdx := range colMapping {
		cards, ok := board.Columns[colName]
		if !ok {
			continue
		}
		for _, c := range cards {
			kc := KanbanCard{
				ID:          c.ID,
				Title:       c.Title,
				Description: c.Description,
				Branch:      c.Branch,
				PaneID:      c.PaneID,
			}
			// Enrich in-progress cards with live pane data
			if colIdx == 1 && c.PaneID != "" {
				if pc, found := paneByID[c.PaneID]; found {
					kc.State = pc.State
					kc.LiveStatus = pc.LiveStatus
					kc.LiveTool = pc.LiveTool
					if kc.Branch == "" {
						kc.Branch = pc.GitBranch
					}
				}
			}
			kb.Columns[colIdx].Cards = append(kb.Columns[colIdx].Cards, kc)
		}
	}

	return kb
}

func renderKanbanColumns(kb KanbanBoard, selectedCol, selectedRow, width, height int) string {
	totalCards := 0
	for _, col := range kb.Columns {
		totalCards += len(col.Cards)
	}
	if totalCards == 0 {
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

	gap := 2
	colWidth := (width - gap*2) / 3
	if colWidth < 20 {
		colWidth = 20
	}
	contentWidth := colWidth - 4 // border + padding

	selectedBorderColor := lipgloss.Color("117")

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Width(colWidth).
		Align(lipgloss.Center).
		MarginBottom(1)

	var columns []string
	for ci, col := range kb.Columns {
		// Column header
		header := headerStyle.Render(col.Name)

		// Render cards
		var cardViews []string
		for ri, card := range col.Cards {
			isSelected := ci == selectedCol && ri == selectedRow

			var borderColor lipgloss.Color
			if isSelected {
				borderColor = selectedBorderColor
			} else {
				switch ci {
				case 0: // backlog
					borderColor = lipgloss.Color("245")
				case 1: // in-progress
					borderColor = stateBorderColor(card.State)
				case 2: // done
					borderColor = lipgloss.Color("238")
				}
			}

			cardStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Width(colWidth - 2).
				Padding(0, 1)

			var content []string
			switch ci {
			case 0: // backlog: title + description
				title := lipgloss.NewStyle().Bold(true).Render(card.Title)
				content = append(content, title)
				if card.Description != "" {
					wrapped := kanbanWordWrap(card.Description, contentWidth)
					maxLines := height - 6
					if maxLines < 1 {
						maxLines = 1
					}
					if len(wrapped) > maxLines {
						wrapped = wrapped[:maxLines]
					}
					descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
					for _, l := range wrapped {
						content = append(content, descStyle.Render(l))
					}
				}

			case 1: // in-progress: emoji + title + branch + tool/status
				line1 := lipgloss.NewStyle().Bold(true).
					Render(fmt.Sprintf("%s %s", stateEmoji(card.State), card.Title))
				content = append(content, line1)
				if card.Branch != "" {
					content = append(content, lipgloss.NewStyle().
						Foreground(lipgloss.Color("114")).Render(card.Branch))
				}
				if card.LiveTool != "" {
					content = append(content, card.LiveTool)
				} else if card.State != 0 {
					content = append(content, card.State.String())
				}
				if card.LiveStatus != "" {
					wrapped := kanbanWordWrap(card.LiveStatus, contentWidth)
					maxLines := height - len(content) - 6
					if maxLines < 1 {
						maxLines = 1
					}
					if len(wrapped) > maxLines {
						wrapped = wrapped[:maxLines]
					}
					content = append(content, wrapped...)
				}

			case 2: // done: title + branch, dimmed
				dimText := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
				content = append(content, dimText.Bold(true).Render(card.Title))
				if card.Branch != "" {
					content = append(content, dimText.Render(card.Branch))
				}
			}

			cardViews = append(cardViews, cardStyle.Render(strings.Join(content, "\n")))
		}

		colContent := header + "\n" + strings.Join(cardViews, "\n")
		colStyle := lipgloss.NewStyle().Width(colWidth)
		columns = append(columns, colStyle.Render(colContent))
	}

	gapStr := strings.Repeat(" ", gap)
	return lipgloss.JoinHorizontal(lipgloss.Top, columns[0], gapStr, columns[1], gapStr, columns[2])
}

