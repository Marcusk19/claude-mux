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
func IsClaudePane(p PaneInfo) bool {
	if strings.Contains(p.PaneTitle, "Claude Code") {
		return true
	}
	if semverRe.MatchString(p.PaneCommand) {
		return true
	}
	return false
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
