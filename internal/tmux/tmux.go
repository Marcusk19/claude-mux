package tmux

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// PaneInfo holds data from tmux list-panes.
type PaneInfo struct {
	SessionName string
	WindowIndex string
	PaneIndex   string
	PaneTitle   string
	PaneCommand string
	PanePath    string
	PaneID      string
}

const delimiter = "%%DELIM%%"

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ListPanes returns all tmux panes across all sessions.
func ListPanes() ([]PaneInfo, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{window_index}",
		"#{pane_index}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_id}",
	}, delimiter)

	out, err := exec.Command("tmux", "list-panes", "-a", "-F", format).Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}

	var panes []PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, delimiter)
		if len(parts) != 7 {
			continue
		}
		panes = append(panes, PaneInfo{
			SessionName: parts[0],
			WindowIndex: parts[1],
			PaneIndex:   parts[2],
			PaneTitle:   parts[3],
			PaneCommand: parts[4],
			PanePath:    parts[5],
			PaneID:      parts[6],
		})
	}
	return panes, nil
}

// IsClaudePane checks if a pane is running Claude Code.
// The pane title persists after a process exits, so we can't rely on it alone.
// We require either a semver command (Claude Code's process name) or both
// a "Claude Code" title and a non-shell command (ruling out exited sessions).
func IsClaudePane(p PaneInfo) bool {
	if semverRe.MatchString(p.PaneCommand) {
		return true
	}
	if strings.Contains(p.PaneTitle, "Claude Code") && !isShell(p.PaneCommand) {
		return true
	}
	return false
}

func isShell(cmd string) bool {
	switch cmd {
	case "zsh", "bash", "sh", "fish", "dash", "ksh", "tcsh", "csh":
		return true
	}
	return false
}

// ListWindowPanes returns all tmux panes in a specific window.
func ListWindowPanes(sessionName, windowIndex string) ([]PaneInfo, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{window_index}",
		"#{pane_index}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_id}",
	}, delimiter)

	target := fmt.Sprintf("%s:%s", sessionName, windowIndex)
	out, err := exec.Command("tmux", "list-panes", "-t", target, "-F", format).Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes -t %s: %w", target, err)
	}

	var panes []PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, delimiter)
		if len(parts) != 7 {
			continue
		}
		panes = append(panes, PaneInfo{
			SessionName: parts[0],
			WindowIndex: parts[1],
			PaneIndex:   parts[2],
			PaneTitle:   parts[3],
			PaneCommand: parts[4],
			PanePath:    parts[5],
			PaneID:      parts[6],
		})
	}
	return panes, nil
}

// CurrentWindow returns the session name and window index of the current tmux window.
func CurrentWindow() (sessionName, windowIndex string, err error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}"+delimiter+"#{window_index}").Output()
	if err != nil {
		return "", "", fmt.Errorf("tmux display-message: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), delimiter)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected tmux output: %s", string(out))
	}
	return parts[0], parts[1], nil
}

// SelectPane switches tmux focus to the given pane.
func SelectPane(p PaneInfo) error {
	target := fmt.Sprintf("%s:%s.%s", p.SessionName, p.WindowIndex, p.PaneIndex)

	if err := exec.Command("tmux", "select-window", "-t", target).Run(); err != nil {
		return fmt.Errorf("select-window: %w", err)
	}
	if err := exec.Command("tmux", "select-pane", "-t", target).Run(); err != nil {
		return fmt.Errorf("select-pane: %w", err)
	}
	// switch-client is needed when the pane is in a different tmux session
	_ = exec.Command("tmux", "switch-client", "-t", p.SessionName).Run()
	return nil
}
