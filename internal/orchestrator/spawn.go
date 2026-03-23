package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SpawnOpts configures a subagent spawn.
type SpawnOpts struct {
	Task         string   // required: task description
	Context      string   // optional: additional context
	Files        []string // optional: file paths to include in task file
	WorktreePath string   // optional: reuse an existing worktree instead of creating one
	BranchName   string   // optional: branch name for the existing worktree
}

// Spawn creates a worktree, writes a task file, opens a tmux pane with claude, and saves state.
// Returns the task ID.
func Spawn(opts SpawnOpts) (string, error) {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return "", fmt.Errorf("resolving orchestrator ID: %w", err)
	}

	repoRoot, err := gitRepoRoot(".")
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
		// Create a new worktree
		repoName := filepath.Base(repoRoot)
		branchName = "worktree/" + taskID
		worktreeDir := filepath.Join(filepath.Dir(repoRoot), repoName+"-wt-"+taskID)

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

	// Open tmux pane with claude
	paneID, err := openClaudePane(absWorktree)
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
	}
	if err := writeState(state); err != nil {
		return "", fmt.Errorf("writing state: %w", err)
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

func openClaudePane(worktreeDir string) (string, error) {
	prompt := "Read .claude-mux/task.md and complete the task. Commit your changes when done."
	// Write prompt to a file to avoid shell quoting issues
	promptFile := filepath.Join(worktreeDir, ".claude-mux", "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0o644); err != nil {
		return "", fmt.Errorf("writing prompt file: %w", err)
	}

	// Launch claude interactively with the prompt as a positional argument.
	// Using --dangerously-skip-permissions for autonomous subagent work.
	shellCmd := `claude --dangerously-skip-permissions "$(cat .claude-mux/prompt.txt)"`

	out, err := exec.Command(
		"tmux", "split-window", "-v",
		"-c", worktreeDir,
		"-P", "-F", "#{pane_id}",
		shellCmd,
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRepoRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
