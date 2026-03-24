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

// State represents the running state of the Command Center.
type State struct {
	PaneID      string    `json:"pane_id"`
	WindowID    string    `json:"window_id"`
	TmuxSession string    `json:"tmux_session"`
	RepoRoot    string    `json:"repo_root"`
	CreatedAt   time.Time `json:"created_at"`
}

// stateFilePath returns the path to the CC state file.
func stateFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "command-center.json")
}

// Start creates a hidden tmux window running Claude Code with the CC system prompt.
// If already running, returns the existing state.
func Start(repoRoot string) (*State, error) {
	existing, _ := Status()
	if existing != nil {
		return existing, nil
	}

	// Get current tmux session name
	sessionOut, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return nil, fmt.Errorf("get tmux session: %w", err)
	}
	sessionName := strings.TrimSpace(string(sessionOut))

	// Create a new hidden window
	out, err := exec.Command("tmux", "new-window", "-d", "-n", "command-center",
		"-c", repoRoot, "-P", "-F", "#{pane_id} #{window_id}").Output()
	if err != nil {
		return nil, fmt.Errorf("create window: %w", err)
	}

	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected new-window output: %s", string(out))
	}
	paneID := parts[0]
	windowID := parts[1]

	// Send the claude command to the new pane
	prompt := strings.ReplaceAll(SystemPrompt(), "'", "'\\''")
	cmd := fmt.Sprintf("claude --dangerously-skip-permissions --append-system-prompt '%s'", prompt)
	err = exec.Command("tmux", "send-keys", "-t", paneID, cmd, "Enter").Run()
	if err != nil {
		// Clean up the window we created
		_ = exec.Command("tmux", "kill-pane", "-t", paneID).Run()
		return nil, fmt.Errorf("send claude command: %w", err)
	}

	state := &State{
		PaneID:      paneID,
		WindowID:    windowID,
		TmuxSession: sessionName,
		RepoRoot:    repoRoot,
		CreatedAt:   time.Now(),
	}

	if err := writeState(state); err != nil {
		return nil, fmt.Errorf("write state: %w", err)
	}

	return state, nil
}

// Stop kills the CC pane and removes the state file.
func Stop() error {
	state, _ := Status()
	if state == nil {
		// Already stopped, just clean up state file if it exists
		os.Remove(stateFilePath())
		return nil
	}

	err := exec.Command("tmux", "kill-pane", "-t", state.PaneID).Run()
	if err != nil {
		// Pane might already be dead, that's fine
	}

	os.Remove(stateFilePath())
	return nil
}

// Status reads the state file and verifies the pane still exists.
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

	// Verify pane still exists
	err = exec.Command("tmux", "display-message", "-t", state.PaneID, "-p", "").Run()
	if err != nil {
		// Pane is dead, clean up stale state
		os.Remove(stateFilePath())
		return nil, nil
	}

	return &state, nil
}

// Open shows the CC window in a tmux popup.
func Open(width, height string) error {
	state, err := Status()
	if err != nil {
		return err
	}
	if state == nil {
		return fmt.Errorf("command center is not running")
	}

	popupCmd := fmt.Sprintf("tmux attach -t %s \\; select-window -t %s",
		state.TmuxSession, state.WindowID)

	return exec.Command("tmux", "display-popup", "-E",
		"-w", width, "-h", height, popupCmd).Run()
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
