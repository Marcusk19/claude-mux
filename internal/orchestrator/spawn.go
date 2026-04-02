package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcusk19/claude-mux/internal/container"
	"github.com/Marcusk19/claude-mux/internal/kanban"
	"github.com/Marcusk19/claude-mux/devcontainer"
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
	Sandbox      bool     // run the subagent inside a sandboxed container
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

	// Write orchestrator metadata files for hook-driven completion
	board, boardErr := kanban.LoadBoard(repoRoot)
	if boardErr == nil {
		taskDir := filepath.Join(absWorktree, ".claude-mux")
		os.MkdirAll(taskDir, 0o755)
		os.WriteFile(filepath.Join(taskDir, "orchestrator-pane"), []byte(board.OrchestratorPaneID), 0o644)
		os.WriteFile(filepath.Join(taskDir, "board-path"), []byte(board.Path), 0o644)
	}

	// Build sandbox image if needed
	var containerName string
	if opts.Sandbox {
		runtime, err := container.DetectRuntime()
		if err != nil {
			return "", fmt.Errorf("sandbox requires docker or podman: %w", err)
		}
		if err := container.EnsureImage(runtime, devcontainer.Assets); err != nil {
			return "", fmt.Errorf("building sandbox image: %w", err)
		}
		containerName = "claude-mux-" + taskID
	}

	// Open tmux pane with claude
	paneID, err := openClaudePane(absWorktree, orchID, opts.Sandbox, containerName)
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
		ContainerName:  containerName,
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

func openClaudePane(worktreeDir string, orchID string, sandbox bool, containerName string) (string, error) {
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
	var shellCmd string
	if sandbox {
		shellCmd = buildSandboxCommand(worktreeDir, containerName)
	} else {
		// Launch claude directly — using --dangerously-skip-permissions for autonomous subagent work.
		shellCmd = `claude --dangerously-skip-permissions --append-system-prompt "$(cat .claude-mux/system-prompt.txt)" "$(cat .claude-mux/prompt.txt)"`
	}

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

// buildSandboxCommand constructs a "docker run ..." shell command for a sandboxed subagent.
func buildSandboxCommand(worktreeDir string, containerName string) string {
	runtime, _ := container.DetectRuntime()

	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "claude-mux")

	// Write an entrypoint script that runs inside the container.
	// Starts as root (for iptables), then drops to the node user (UID 1000)
	// for claude because --dangerously-skip-permissions refuses to run as root.
	entryScript := `#!/bin/sh
set -e
/usr/local/bin/init-firewall.sh
# Ensure node user can write to mounted volumes
chown -R node:node /workspace 2>/dev/null || true
chown -R node:node /home/node/.claude /home/node/.claude.json 2>/dev/null || true
chown -R node:node /home/node/.config 2>/dev/null || true
# Drop to node user for claude (refuses --dangerously-skip-permissions as root).
# --bare skips hooks/onboarding setup, uses env-based auth directly.
# -p skips the workspace trust dialog and exits after completion.
export HOME=/home/node
exec su -s /bin/sh node -c 'cd /workspace && exec claude -p --bare --dangerously-skip-permissions \
  --append-system-prompt "$(cat .claude-mux/system-prompt.txt)" \
  "$(cat .claude-mux/prompt.txt)"'
`
	entryScriptPath := filepath.Join(worktreeDir, ".claude-mux", "sandbox-entry.sh")
	os.WriteFile(entryScriptPath, []byte(entryScript), 0o755)

	// Run as root inside the container. The container IS the sandbox —
	// the firewall provides network isolation and the container boundary
	// provides filesystem isolation. Root is needed for iptables setup.
	cfg := container.ContainerConfig{
		Image:       container.ImageName,
		Name:        containerName,
		Remove:      true,
		Interactive: true,
		WorkDir:     "/workspace",
		User:        "0:0",
		Caps:        []string{"NET_ADMIN", "NET_RAW"},
		Command:     "/workspace/.claude-mux/sandbox-entry.sh",
		Mounts: []container.Mount{
			{Source: worktreeDir, Target: "/workspace"},
			{Source: cacheDir, Target: cacheDir},
		},
		EnvVars: map[string]string{
			"CLAUDE_MUX_CACHE": cacheDir,
		},
	}

	// Pass auth credentials. Support both Anthropic API key and Vertex AI (Google Cloud).
	authEnvVars := []string{
		"ANTHROPIC_API_KEY",
		// Vertex AI / Google Cloud auth
		"CLAUDE_CODE_USE_VERTEX",
		"ANTHROPIC_VERTEX_PROJECT_ID",
		"CLOUD_ML_REGION",
		"CLOUD_ML_PROJECT_ID",
		"GOOGLE_APPLICATION_CREDENTIALS",
	}
	for _, key := range authEnvVars {
		if val := os.Getenv(key); val != "" {
			cfg.EnvVars[key] = val
		}
	}

	// Mount Google Cloud ADC credentials for Vertex AI auth.
	// Not read-only because the node user needs read access after chown.
	gcloudConfig := filepath.Join(home, ".config", "gcloud")
	if info, err := os.Stat(gcloudConfig); err == nil && info.IsDir() {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: gcloudConfig, Target: "/home/node/.config/gcloud",
		})
	}

	// Mount claude config for session state
	claudeConfig := filepath.Join(home, ".claude")
	if info, err := os.Stat(claudeConfig); err == nil && info.IsDir() {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: claudeConfig, Target: "/home/node/.claude",
		})
	}

	// Mount .claude.json (onboarding state, theme, etc.)
	claudeJSON := filepath.Join(home, ".claude.json")
	if _, err := os.Stat(claudeJSON); err == nil {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: claudeJSON, Target: "/home/node/.claude.json",
		})
	}

	// Mount gitconfig read-only
	gitconfig := filepath.Join(home, ".gitconfig")
	if _, err := os.Stat(gitconfig); err == nil {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: gitconfig, Target: "/home/node/.gitconfig", ReadOnly: true,
		})
	}

	// Mount SSH keys read-only if they exist
	sshDir := filepath.Join(home, ".ssh")
	if _, err := os.Stat(sshDir); err == nil {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: sshDir, Target: "/home/node/.ssh", ReadOnly: true,
		})
	}

	return container.BuildShellCommand(runtime, cfg)
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
