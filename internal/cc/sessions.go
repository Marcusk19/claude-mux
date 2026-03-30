package cc

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcusk19/claude-mux/internal/session"
)

// SessionEntry represents a single Claude session in the output JSON.
type SessionEntry struct {
	PaneID        string       `json:"pane_id"`
	PaneTarget    string       `json:"pane_target"`
	WindowName    string       `json:"window_name"`
	ProjectPath   string       `json:"project_path"`
	State         string       `json:"state"`
	LiveStatus    string       `json:"live_status,omitempty"`
	LiveTool      string       `json:"live_tool,omitempty"`
	Summary       string       `json:"summary,omitempty"`
	InitialPrompt string       `json:"initial_prompt,omitempty"`
	GitBranch     string       `json:"git_branch,omitempty"`
	MessageCount  int          `json:"message_count"`
	LastActivity  time.Time    `json:"last_activity"`
	Pinned        bool         `json:"pinned"`
	Capture       *CaptureData `json:"capture,omitempty"`
}

// CaptureData holds captured pane output.
type CaptureData struct {
	LastLines  string    `json:"last_lines"`
	CapturedAt time.Time `json:"captured_at"`
}

// SessionsState is the top-level structure written to the JSON file.
type SessionsState struct {
	GeneratedAt  time.Time      `json:"generated_at"`
	SessionCount int            `json:"session_count"`
	Sessions     []SessionEntry `json:"sessions"`
}

// SessionsOpts configures DiscoverAndWrite behavior.
type SessionsOpts struct {
	Capture      bool
	CaptureLines int
}

// sessionEntryFrom converts a ClaudeSession to a SessionEntry.
func sessionEntryFrom(cs session.ClaudeSession) SessionEntry {
	paneTarget := fmt.Sprintf("%s:%s.%s", cs.Pane.SessionName, cs.Pane.WindowIndex, cs.Pane.PaneIndex)
	return SessionEntry{
		PaneID:        cs.Pane.PaneID,
		PaneTarget:    paneTarget,
		WindowName:    cs.Pane.WindowName,
		ProjectPath:   cs.ProjectPath,
		State:         cs.State.String(),
		LiveStatus:    cs.LiveStatus,
		LiveTool:      cs.LiveTool,
		Summary:       cs.Summary,
		InitialPrompt: cs.InitialPrompt,
		GitBranch:     cs.GitBranch,
		MessageCount:  cs.MessageCount,
		LastActivity:  cs.LastActivity,
		Pinned:        cs.Pinned,
	}
}

// capturePaneOutput runs tmux capture-pane and returns the trimmed output.
func capturePaneOutput(paneID string, lines int) (string, error) {
	out, err := exec.Command("tmux", "capture-pane", "-t", paneID, "-p", "-S", fmt.Sprintf("-%d", lines)).Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane %s: %w", paneID, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// DiscoverAndWrite discovers all active Claude sessions, optionally captures
// pane output, and writes the result to the sessions state file.
func DiscoverAndWrite(opts SessionsOpts) (*SessionsState, error) {
	sessions, err := session.DiscoverSessions()
	if err != nil {
		return nil, fmt.Errorf("discover sessions: %w", err)
	}

	entries := make([]SessionEntry, 0, len(sessions))
	for _, cs := range sessions {
		entry := sessionEntryFrom(cs)
		if opts.Capture {
			lines := opts.CaptureLines
			if lines <= 0 {
				lines = 20
			}
			output, err := capturePaneOutput(cs.Pane.PaneID, lines)
			if err == nil && output != "" {
				entry.Capture = &CaptureData{
					LastLines:  output,
					CapturedAt: time.Now(),
				}
			}
		}
		entries = append(entries, entry)
	}

	state := &SessionsState{
		GeneratedAt:  time.Now(),
		SessionCount: len(entries),
		Sessions:     entries,
	}

	if err := writeSessionsState(state); err != nil {
		return nil, fmt.Errorf("write sessions state: %w", err)
	}

	return state, nil
}

// ReadSessionsState reads and unmarshals the sessions state file.
func ReadSessionsState() (*SessionsState, error) {
	data, err := os.ReadFile(sessionsStatePath())
	if err != nil {
		return nil, err
	}
	var state SessionsState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// sessionsStatePath returns the path to the sessions state JSON file.
func sessionsStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "cc-sessions.json")
}

// SessionsStatePath is the exported version for use by main.
func SessionsStatePath() string {
	return sessionsStatePath()
}

func writeSessionsState(state *SessionsState) error {
	path := sessionsStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
