package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Marcusk19/claude-mux/devcontainer"
	"github.com/Marcusk19/claude-mux/internal/cc"
	"github.com/Marcusk19/claude-mux/internal/container"
	"github.com/Marcusk19/claude-mux/internal/hook"
	"github.com/Marcusk19/claude-mux/internal/kanban"
	"github.com/Marcusk19/claude-mux/internal/orchestrator"
	"github.com/Marcusk19/claude-mux/internal/tmux"
	"github.com/Marcusk19/claude-mux/internal/ui"
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
		case "swarm":
			runSwarm()
			return
		case "board":
			runBoard()
			return
		case "cc":
			runCC()
			return
		case "plan":
			runPlan()
			return
		case "sandbox-build":
			runSandboxBuild()
			return
		case "--help", "-h", "help":
			printUsage()
			return
		}
	}

	// Default: launch TUI
	runTUI()
}

func printUsage() {
	fmt.Println(`Usage: claude-mux [command] [flags]

Commands:
  (none)      Launch the TUI overlay (default)
  hook        Process a Claude Code hook event from stdin
              Usage: claude-mux hook <event>

  spawn       Spawn a subagent in a new worktree
              --task string       task description (required)
              --context string    additional context
              --file string       comma-separated file paths to include
              --worktree string   reuse an existing worktree path
              --branch string     branch name for the existing worktree
              --card-id string    kanban card ID to move to in-progress
              --sandbox           run in a sandboxed container (firewall + isolation)

  status      Show status of all subagents

  collect     Collect results from completed subagents
              --task-id string    collect a specific subagent
              --merge             merge completed branches
              --cleanup           remove worktrees after collecting

  cleanup     Remove subagent worktrees and state
              --task-id string    clean up a specific subagent
              --force             remove running subagents too

  swarm       Coordinate multiple subagents for a task
              --task string       task description (required)
              --context string    additional context
              --file string       comma-separated file paths to include
              --auto-merge        auto-merge completed branches
              --max-agents int    max concurrent subagents (default 3)
              --sandbox           run subagents in sandboxed containers

  board       Update kanban board cards
              Usage: claude-mux board update --card-id <id> --column <col>

  cc          Command Center management
              Usage: claude-mux cc <start|stop|status|open|sessions>
              sessions    Discover active Claude sessions and write JSON state
                --capture         capture last N lines from each pane
                --capture-lines N number of lines to capture (default 20)
                --json            output full JSON to stdout

  plan        Interactive PRD planning that can launch a swarm
              --task string       initial task idea (optional)
              --context string    additional context
              --file string       comma-separated file paths to include
              --auto-merge        auto-merge completed branches when swarm executes
              --max-agents int    max concurrent subagents (default 3)
              --sandbox           run subagents in sandboxed containers

  sandbox-build  Pre-build the sandbox container image
              --force             force rebuild even if image is up to date

Flags:
  --help, -h  Show this help message`)
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
	worktree := fs.String("worktree", "", "reuse an existing worktree path")
	branch := fs.String("branch", "", "branch name for the existing worktree")
	cardID := fs.String("card-id", "", "kanban card ID to move to in-progress")
	sandbox := fs.Bool("sandbox", false, "run subagent in a sandboxed container")
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
		Task:         *task,
		Context:      *context,
		Files:        files,
		WorktreePath: *worktree,
		BranchName:   *branch,
		CardID:       *cardID,
		Sandbox:      *sandbox,
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

func runSwarm() {
	fs := flag.NewFlagSet("swarm", flag.ExitOnError)
	task := fs.String("task", "", "task description (required)")
	context := fs.String("context", "", "additional context")
	filesFlag := fs.String("file", "", "comma-separated file paths to include")
	autoMerge := fs.Bool("auto-merge", false, "auto-merge completed branches")
	maxAgents := fs.Int("max-agents", 3, "max concurrent subagents")
	sandbox := fs.Bool("sandbox", false, "run subagents in sandboxed containers")
	fs.Parse(os.Args[2:])

	if *task == "" {
		fmt.Fprintf(os.Stderr, "error: --task is required\n")
		os.Exit(1)
	}

	var files []string
	if *filesFlag != "" {
		files = strings.Split(*filesFlag, ",")
	}

	if err := orchestrator.Swarm(orchestrator.SwarmOpts{
		Task:      *task,
		Context:   *context,
		Files:     files,
		AutoMerge: *autoMerge,
		MaxAgents: *maxAgents,
		Sandbox:   *sandbox,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "swarm error: %v\n", err)
		os.Exit(1)
	}
}

func runPlan() {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	task := fs.String("task", "", "initial task idea (optional)")
	context := fs.String("context", "", "additional context")
	filesFlag := fs.String("file", "", "comma-separated file paths to include")
	autoMerge := fs.Bool("auto-merge", false, "auto-merge completed branches when swarm executes")
	maxAgents := fs.Int("max-agents", 3, "max concurrent subagents for swarm execution")
	sandbox := fs.Bool("sandbox", false, "run subagents in sandboxed containers")
	fs.Parse(os.Args[2:])

	var files []string
	if *filesFlag != "" {
		files = strings.Split(*filesFlag, ",")
	}

	if err := orchestrator.Plan(orchestrator.PlanOpts{
		Task:      *task,
		Context:   *context,
		Files:     files,
		AutoMerge: *autoMerge,
		MaxAgents: *maxAgents,
		Sandbox:   *sandbox,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "plan error: %v\n", err)
		os.Exit(1)
	}
}

func runBoard() {
	if len(os.Args) < 3 || os.Args[2] != "update" {
		fmt.Fprintf(os.Stderr, "usage: claude-mux board update --card-id <id> --column <col>\n")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("board-update", flag.ExitOnError)
	cardID := fs.String("card-id", "", "card ID to move")
	column := fs.String("column", "", "target column (backlog, in-progress, done)")
	fs.Parse(os.Args[3:])

	if *cardID == "" || *column == "" {
		fmt.Fprintf(os.Stderr, "error: --card-id and --column are required\n")
		os.Exit(1)
	}

	repoRoot, err := orchestrator.RepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	board, err := kanban.LoadBoard(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := kanban.MoveCard(board, *cardID, *column); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := kanban.SaveBoard(board); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Card %s moved to %s\n", *cardID, *column)
}

func runCC() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: claude-mux cc <start|stop|status|open>\n")
		os.Exit(1)
	}

	switch os.Args[2] {
	case "start":
		repoRoot, err := orchestrator.RepoRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		state, err := cc.Start(repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(state.PaneID)
	case "stop":
		if err := cc.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Command Center stopped.")
	case "status":
		state, err := cc.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if state == nil {
			fmt.Println("Command Center is not running.")
			return
		}
		fmt.Printf("Pane:    %s\n", state.PaneID)
		fmt.Printf("Uptime:  %s\n", time.Since(state.CreatedAt).Truncate(time.Second))
		fmt.Printf("Repo:    %s\n", state.RepoRoot)
	case "open":
		repoRoot, err := orchestrator.RepoRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		width := tmuxOption("@claude-mux-width", "80%")
		height := tmuxOption("@claude-mux-height", "70%")
		if _, err := cc.EnsureRunning(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := cc.Open(width, height); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "sessions":
		fs := flag.NewFlagSet("cc-sessions", flag.ExitOnError)
		capture := fs.Bool("capture", false, "capture last N lines from each pane")
		captureLines := fs.Int("capture-lines", 20, "number of lines to capture")
		jsonOut := fs.Bool("json", false, "output full JSON to stdout")
		fs.Parse(os.Args[3:])

		state, err := cc.DiscoverAndWrite(cc.SessionsOpts{
			Capture:      *capture,
			CaptureLines: *captureLines,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if *jsonOut {
			data, _ := json.MarshalIndent(state, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("Found %d active session(s)\n", state.SessionCount)
			for _, s := range state.Sessions {
				fmt.Printf("  %s  %-10s  %s  %s\n", s.PaneID, s.State, s.GitBranch, s.Summary)
			}
			fmt.Printf("\nState written to %s\n", cc.SessionsStatePath())
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown cc subcommand: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func tmuxOption(name, defaultVal string) string {
	out, err := exec.Command("tmux", "show-option", "-gqv", name).Output()
	if err != nil {
		return defaultVal
	}
	val := strings.TrimSpace(string(out))
	if val == "" {
		return defaultVal
	}
	return val
}

func runSandboxBuild() {
	fs := flag.NewFlagSet("sandbox-build", flag.ExitOnError)
	force := fs.Bool("force", false, "force rebuild even if image is up to date")
	fs.Parse(os.Args[2:])

	runtime, err := container.DetectRuntime()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *force {
		err = container.BuildImage(runtime, devcontainer.Assets)
	} else {
		err = container.EnsureImage(runtime, devcontainer.Assets)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Sandbox image ready:", container.ImageName)
}

func runTUI() {
	kanbanSession := os.Getenv("CLAUDE_MUX_SESSION")
	kanbanWindow := os.Getenv("CLAUDE_MUX_WINDOW")

	// Fallback: query tmux directly
	if kanbanSession == "" || kanbanWindow == "" {
		if s, w, err := tmux.CurrentWindow(); err == nil {
			kanbanSession = s
			kanbanWindow = w
		}
	}

	m := ui.NewModel(kanbanSession, kanbanWindow)
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
