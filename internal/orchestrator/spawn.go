package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcusk19/claude-mux/internal/kanban"
	"github.com/Marcusk19/claude-mux/internal/openshell"
)

const subagentSystemPromptTmpl = `You are an implementation agent in a multi-agent swarm. You have been assigned a specific subtask.

## Your Role

- Read your task file at ` + "`.claude-mux/task.md`" + ` for your assignment
- Implement the task completely — write clean, working code
- Run tests if applicable (` + "`make build`" + `, ` + "`go test ./...`" + `, etc.)
- Commit your changes with a clear commit message when done
- Stay focused on your subtask — do not modify files outside your assignment unless necessary

## Guidelines

- Keep changes minimal and focused
- Do not refactor unrelated code
- If you encounter a blocker, commit what you have with a note in the commit message
- Your work will be validated by a separate agent after completion
`

// SpawnOpts configures a subagent spawn.
type SpawnOpts struct {
	Task         string   // required: task description
	Context      string   // optional: additional context
	Files        []string // optional: file paths to include in task file
	WorktreePath string   // optional: reuse an existing worktree instead of creating one
	BranchName   string   // optional: branch name for the existing worktree
	CardID       string   // optional: kanban card ID to move to in-progress
	Sandbox      bool     // run the subagent inside an openshell sandbox
	Provider     string   // optional: openshell provider name (empty = use openshell default)
	RepoDir      string   // optional: git repo directory (defaults to cwd)
}

// Spawn creates a worktree, writes a task file, opens a tmux pane with claude, and saves state.
// Returns the task ID.
func Spawn(opts SpawnOpts) (string, error) {
	// Enforce sandbox when running from Command Center context
	if os.Getenv("CLAUDE_MUX_CC") == "1" && !opts.Sandbox {
		return "", fmt.Errorf("sandbox is required when spawning from the Command Center (use --sandbox)")
	}

	orchID, err := resolveOrchestratorID()
	if err != nil {
		return "", fmt.Errorf("resolving orchestrator ID: %w", err)
	}

	// Set CLAUDE_MUX_SESSION for the orchestrator process itself
	os.Setenv("CLAUDE_MUX_SESSION", orchID)

	repoDir := opts.RepoDir
	if repoDir == "" {
		repoDir = "."
	}
	repoRoot, err := gitRepoRoot(repoDir)
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	taskID := generateTaskID()

	var absWorktree, branchName string

	if opts.WorktreePath != "" {
		// Reuse an existing worktree
		absWorktree, err = filepath.Abs(opts.WorktreePath)
		if err != nil {
			absWorktree = opts.WorktreePath
		}
		branchName = opts.BranchName
		if branchName == "" {
			// Detect branch from the worktree
			out, err := exec.Command("git", "-C", absWorktree, "rev-parse", "--abbrev-ref", "HEAD").Output()
			if err != nil {
				return "", fmt.Errorf("detecting branch in worktree: %w", err)
			}
			branchName = strings.TrimSpace(string(out))
		}
	} else {
		// Create a new worktree inside .claude/worktrees/
		repoName := filepath.Base(repoRoot)
		branchName = "worktree/" + taskID
		worktreeParent := filepath.Join(repoRoot, ".claude", "worktrees")
		if err := os.MkdirAll(worktreeParent, 0o755); err != nil {
			return "", fmt.Errorf("creating worktree directory: %w", err)
		}
		worktreeDir := filepath.Join(worktreeParent, repoName+"-wt-"+taskID)

		out, err := exec.Command("git", "worktree", "add", worktreeDir, "-b", branchName).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("creating worktree: %s: %s", err, strings.TrimSpace(string(out)))
		}

		absWorktree, err = filepath.Abs(worktreeDir)
		if err != nil {
			absWorktree = worktreeDir
		}
	}

	// Write task file
	if err := writeTaskFile(absWorktree, opts); err != nil {
		return "", fmt.Errorf("writing task file: %w", err)
	}

	// Write orchestrator metadata files for hook-driven completion
	board, boardErr := kanban.LoadBoard(repoRoot)
	if boardErr == nil {
		taskDir := filepath.Join(absWorktree, ".claude-mux")
		os.MkdirAll(taskDir, 0o755)
		os.WriteFile(filepath.Join(taskDir, "orchestrator-pane"), []byte(board.OrchestratorPaneID), 0o644)
		os.WriteFile(filepath.Join(taskDir, "board-path"), []byte(board.Path), 0o644)
	}

	// Verify openshell is available if sandbox is requested
	var sandboxName string
	if opts.Sandbox {
		if !openshell.Available() {
			return "", fmt.Errorf("sandbox requires openshell (not found in PATH)")
		}
		sandboxName = "claude-mux-" + taskID
	}

	// Open tmux pane with claude
	paneID, err := openClaudePane(absWorktree, orchID, taskID, opts.Sandbox, sandboxName, opts.Provider)
	if err != nil {
		return "", fmt.Errorf("opening tmux pane: %w", err)
	}

	// Save state
	state := SubagentState{
		TaskID:         taskID,
		OrchestratorID: orchID,
		Task:           opts.Task,
		WorktreePath:   absWorktree,
		BranchName:     branchName,
		RepoRoot:       repoRoot,
		PaneID:         paneID,
		Status:         "running",
		CreatedAt:      time.Now(),
		Sandboxed:      opts.Sandbox,
		SandboxName:    sandboxName,
	}
	if err := writeState(state); err != nil {
		return "", fmt.Errorf("writing state: %w", err)
	}

	// Move kanban card to in-progress
	if opts.CardID != "" && boardErr == nil {
		// Reload board in case it was modified
		board, err = kanban.LoadBoardFromPath(board.Path)
		if err == nil {
			card, _ := kanban.FindCardByID(board, opts.CardID)
			if card != nil {
				card.TaskID = taskID
				card.PaneID = paneID
				card.Branch = branchName
				kanban.MoveCard(board, opts.CardID, "in-progress")
				kanban.SaveBoard(board)
			}
		}
	}

	return taskID, nil
}

func writeTaskFile(worktreeDir string, opts SpawnOpts) error {
	taskDir := filepath.Join(worktreeDir, ".claude-mux")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# Task\n\n")
	b.WriteString(opts.Task)
	b.WriteString("\n")

	if opts.Context != "" {
		b.WriteString("\n## Context\n\n")
		b.WriteString(opts.Context)
		b.WriteString("\n")
	}

	for _, f := range opts.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		b.WriteString("\n## File: ")
		b.WriteString(f)
		b.WriteString("\n\n```\n")
		b.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	return os.WriteFile(filepath.Join(taskDir, "task.md"), []byte(b.String()), 0o644)
}

func paneExists(paneID string) bool {
	err := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{pane_id}").Run()
	return err == nil
}

func openClaudePane(worktreeDir string, orchID string, taskID string, sandbox bool, sandboxName string, provider string) (string, error) {
	prompt := "Read .claude-mux/task.md and complete the task. Commit your changes when done."
	// Write prompt to a file to avoid shell quoting issues
	promptFile := filepath.Join(worktreeDir, ".claude-mux", "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0o644); err != nil {
		return "", fmt.Errorf("writing prompt file: %w", err)
	}

	// Write system prompt file
	systemPromptFile := filepath.Join(worktreeDir, ".claude-mux", "system-prompt.txt")
	if err := os.WriteFile(systemPromptFile, []byte(subagentSystemPromptTmpl), 0o644); err != nil {
		return "", fmt.Errorf("writing system prompt file: %w", err)
	}

	// Build the shell command to run in the pane
	shellCmd := buildShellCmd(orchID, taskID, worktreeDir, sandbox, sandboxName, provider)

	// Always spawn subagents in a new tmux window, never within the orchestrator's session.
	// This keeps the orchestrator/command center isolated from subagent panes.
	windowName := filepath.Base(worktreeDir)
	newWindowArgs := []string{
		"new-window",
		"-d",           // don't switch to the new window
		"-n", windowName,
		"-c", worktreeDir,
		"-P", "-F", "#{pane_id}",
		shellCmd,
	}

	out, err := exec.Command("tmux", newWindowArgs...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// buildShellCmd constructs the shell command string for a subagent pane.
func buildShellCmd(orchID, taskID, worktreeDir string, sandbox bool, sandboxName string, provider string) string {
	sessionEnv := orchID + "/" + taskID
	if sandbox {
		sandboxCmd := openshell.BuildAutonomousCommand(sandboxName, worktreeDir, provider)
		sandboxLog := filepath.Join(worktreeDir, ".claude-mux", "sandbox.log")
		return fmt.Sprintf("%s 2>&1 | tee %s", sandboxCmd, sandboxLog)
	}
	// Launch claude directly — using --dangerously-skip-permissions for autonomous subagent work.
	return fmt.Sprintf(`export CLAUDE_MUX_SESSION=%s && claude --dangerously-skip-permissions --append-system-prompt "$(cat .claude-mux/system-prompt.txt)" "$(cat .claude-mux/prompt.txt)"`, sessionEnv)
}

// RepoRoot returns the git repository root for the current directory.
func RepoRoot() (string, error) {
	return gitRepoRoot(".")
}

func gitRepoRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
