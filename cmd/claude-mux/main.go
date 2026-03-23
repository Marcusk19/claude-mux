package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mkok/claude-mux/internal/hook"
	"github.com/mkok/claude-mux/internal/orchestrator"
	"github.com/mkok/claude-mux/internal/tmux"
	"github.com/mkok/claude-mux/internal/ui"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "hook":
			runHook()
			return
		case "spawn":
			runSpawn()
			return
		case "status":
			runStatus()
			return
		case "collect":
			runCollect()
			return
		case "cleanup":
			runCleanup()
			return
		}
	}

	// Default: launch TUI
	runTUI()
}

func runHook() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: claude-mux hook <event>\n")
		os.Exit(1)
	}
	event := os.Args[2]
	if err := hook.Handle(event); err != nil {
		fmt.Fprintf(os.Stderr, "hook error: %v\n", err)
		os.Exit(1)
	}
}

func runSpawn() {
	fs := flag.NewFlagSet("spawn", flag.ExitOnError)
	task := fs.String("task", "", "task description (required)")
	context := fs.String("context", "", "additional context")
	filesFlag := fs.String("file", "", "comma-separated file paths to include")
	fs.Parse(os.Args[2:])

	if *task == "" {
		fmt.Fprintf(os.Stderr, "error: --task is required\n")
		os.Exit(1)
	}

	var files []string
	if *filesFlag != "" {
		files = strings.Split(*filesFlag, ",")
	}

	taskID, err := orchestrator.Spawn(orchestrator.SpawnOpts{
		Task:    *task,
		Context: *context,
		Files:   files,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(taskID)
}

func runStatus() {
	results, err := orchestrator.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "status error: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(orchestrator.FormatStatus(results))
}

func runCollect() {
	fs := flag.NewFlagSet("collect", flag.ExitOnError)
	taskID := fs.String("task-id", "", "collect a specific subagent")
	merge := fs.Bool("merge", false, "merge completed branches")
	cleanup := fs.Bool("cleanup", false, "remove worktrees after collecting")
	fs.Parse(os.Args[2:])

	results, err := orchestrator.Collect(orchestrator.CollectOpts{
		TaskID:  *taskID,
		Merge:   *merge,
		Cleanup: *cleanup,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect error: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(orchestrator.FormatCollect(results))
}

func runCleanup() {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	taskID := fs.String("task-id", "", "clean up a specific subagent")
	force := fs.Bool("force", false, "remove running subagents too")
	fs.Parse(os.Args[2:])

	if err := orchestrator.Cleanup(orchestrator.CleanupOpts{
		TaskID: *taskID,
		Force:  *force,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Cleanup complete.")
}

func runTUI() {
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
