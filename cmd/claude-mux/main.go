package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mkok/claude-mux/internal/tmux"
	"github.com/mkok/claude-mux/internal/ui"
)

func main() {
	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if model, ok := finalModel.(*ui.Model); ok {
		if selected := model.Selected(); selected != nil {
			if err := tmux.SelectPane(selected.Pane); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to switch pane: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
