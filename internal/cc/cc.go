package cc

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	socketName  = "claude-mux-cc"
	sessionName = "cc"
)

// State represents the running state of the Command Center.
type State struct {
	PaneID    string    `json:"pane_id"`
	RepoRoot  string    `json:"repo_root"`
	CreatedAt time.Time `json:"created_at"`
}

// DefaultRepoRoot returns the default repo root for the CC session.
func DefaultRepoRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "obsidian-git-sync")
}

// stateFilePath returns the path to the CC state file.
func stateFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "command-center.json")
}

// Start creates a detached tmux session on a separate socket running Claude Code
// with the CC system prompt. If already running, returns the existing state.
func Start(repoRoot string) (*State, error) {
	existing, _ := Status()
	if existing != nil {
		return existing, nil
	}

	// Create a detached session on the separate socket
	if err := runTmux("-L", socketName, "new-session", "-d",
		"-s", sessionName, "-c", repoRoot); err != nil {
		return nil, fmt.Errorf("create CC session: %w", err)
	}

	// Write system prompt to a file so it doesn't get echoed in the terminal
	home, _ := os.UserHomeDir()
	promptDir := filepath.Join(home, ".cache", "claude-mux")
	os.MkdirAll(promptDir, 0o755)
	promptFile := filepath.Join(promptDir, "cc-system-prompt.txt")
	if err := os.WriteFile(promptFile, []byte(SystemPrompt()), 0o644); err != nil {
		_ = runTmux("-L", socketName, "kill-session", "-t", sessionName)
		return nil, fmt.Errorf("write system prompt: %w", err)
	}

	// Override TMUX env var in the CC pane to point to the main tmux server,
	// so that tmux commands (and claude-mux status/spawn/etc.) target the
	// correct server instead of the CC's separate socket.
	mainTmux := os.Getenv("TMUX")
	if mainTmux != "" {
		exportCmd := fmt.Sprintf("export TMUX='%s'", mainTmux)
		_ = runTmux("-L", socketName, "send-keys", "-t", sessionName, exportCmd, "Enter")
	}

	// Send the claude command referencing the prompt file
	claudeCmd := fmt.Sprintf(`claude --dangerously-skip-permissions --append-system-prompt "$(cat %s)"`, promptFile)
	if err := runTmux("-L", socketName, "send-keys", "-t", sessionName, claudeCmd, "Enter"); err != nil {
		_ = runTmux("-L", socketName, "kill-session", "-t", sessionName)
		return nil, fmt.Errorf("send claude command: %w", err)
	}

	// Get the pane ID from the new session
	paneID, err := runTmuxOutput("-L", socketName, "display-message",
		"-t", sessionName, "-p", "#{pane_id}")
	if err != nil {
		_ = runTmux("-L", socketName, "kill-session", "-t", sessionName)
		return nil, fmt.Errorf("get pane id: %w", err)
	}

	state := &State{
		PaneID:    paneID,
		RepoRoot:  repoRoot,
		CreatedAt: time.Now(),
	}

	if err := writeState(state); err != nil {
		return nil, fmt.Errorf("write state: %w", err)
	}

	return state, nil
}

// Stop kills the CC session on the separate socket and removes the state file.
func Stop() error {
	// Kill the entire session on the separate socket
	_ = runTmux("-L", socketName, "kill-session", "-t", sessionName)
	os.Remove(stateFilePath())
	return nil
}

// Status checks whether the CC session exists on the separate socket.
// Returns nil if the CC is not running.
func Status() (*State, error) {
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		os.Remove(stateFilePath())
		return nil, nil
	}

	// Verify the session still exists on the separate socket
	err = runTmux("-L", socketName, "has-session", "-t", sessionName)
	if err != nil {
		// Session is dead, clean up stale state
		os.Remove(stateFilePath())
		return nil, nil
	}

	return &state, nil
}

// Open shows the CC session in a tmux popup by attaching to the separate socket.
// If not inside a tmux session, attaches directly instead of using display-popup.
func Open(width, height string) error {
	state, err := Status()
	if err != nil {
		return err
	}
	if state == nil {
		return fmt.Errorf("command center is not running")
	}

	// If not inside tmux, attach directly to the CC session
	if os.Getenv("TMUX") == "" {
		cmd := exec.Command("tmux", "-L", socketName, "attach", "-t", sessionName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	popupCmd := fmt.Sprintf("tmux -L %s attach -t %s", socketName, sessionName)

	return runTmux("display-popup", "-E",
		"-w", width, "-h", height, popupCmd)
}

// EnsureRunning returns the current CC state, starting it if necessary.
func EnsureRunning(repoRoot string) (*State, error) {
	state, err := Status()
	if err != nil {
		return nil, err
	}
	if state != nil {
		return state, nil
	}
	return Start(repoRoot)
}

// runTmux executes a tmux command and returns a descriptive error including
// tmux's stderr output if it fails.
func runTmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return fmt.Errorf("tmux %s: %s", args[0], detail)
		}
		return fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return nil
}

// runTmuxOutput executes a tmux command and returns its stdout, or a
// descriptive error including tmux's stderr.
func runTmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return "", fmt.Errorf("tmux %s: %s", args[0], detail)
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func writeState(state *State) error {
	path := stateFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
