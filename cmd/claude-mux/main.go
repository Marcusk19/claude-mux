package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mkok/claude-mux/internal/hook"
	"github.com/mkok/claude-mux/internal/tmux"
	"github.com/mkok/claude-mux/internal/ui"
)

func main() {
	// Hook subcommand: claude-mux hook <event>
	if len(os.Args) >= 3 && os.Args[1] == "hook" {
		event := os.Args[2]
		if err := hook.Handle(event); err != nil {
			fmt.Fprintf(os.Stderr, "hook error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Default: launch TUI
	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if model, ok := finalModel.(*ui.Model); ok {
		var pane *tmux.PaneInfo
		if selected := model.Selected(); selected != nil {
			pane = &selected.Pane
		} else if p := model.SelectedPane(); p != nil {
			pane = p
		}
		if pane != nil {
			if err := tmux.SelectPane(*pane); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to switch pane: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
