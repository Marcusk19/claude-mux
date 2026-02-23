package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mkok/claude-mux/internal/hook"
	"github.com/mkok/claude-mux/internal/pin"
	"github.com/mkok/claude-mux/internal/tmux"
)

// DiscoverSessions finds all Claude Code panes and enriches them with session data.
func DiscoverSessions() ([]ClaudeSession, error) {
	panes, err := tmux.ListPanes()
	if err != nil {
		return nil, err
	}

	pins := pin.Load()
	pinnedSet := make(map[string]bool, len(pins))
	for _, p := range pins {
		pinnedSet[p] = true
	}

	var sessions []ClaudeSession
	for _, p := range panes {
		if !tmux.IsClaudePane(p) {
			continue
		}

		cs := ClaudeSession{
			Pane:        p,
			State:       inferState(p.PaneTitle),
			ProjectPath: p.PanePath,
		}

		// Try to find session metadata from Claude's project data
		entry, err := findMostRecentSession(p.PanePath)
		if err == nil && entry != nil {
			cs.Summary = entry.Summary
			cs.InitialPrompt = entry.FirstPrompt
			cs.GitBranch = entry.GitBranch
			cs.MessageCount = entry.MessageCount
			if t, err := time.Parse(time.RFC3339Nano, entry.Modified); err == nil {
				cs.Modified = t
			}

			// Try to read JSONL for last activity timestamp
			if entry.FullPath != "" {
				if lastTime, _, err := readJSONLTail(entry.FullPath, 8192); err == nil {
					cs.LastActivity = lastTime
				}

				// If no summary, read the first user prompt from the JSONL as a fallback
				if cs.Summary == "" && cs.InitialPrompt == "" {
					cs.InitialPrompt = readFirstUserPrompt(entry.FullPath)
				}
			}
		}

		// Fall back to modified time if no JSONL activity
		if cs.LastActivity.IsZero() && !cs.Modified.IsZero() {
			cs.LastActivity = cs.Modified
		}

		// Enrich with live hook state if available
		enrichWithHookState(&cs)

		cs.Pinned = pinnedSet[cs.ProjectPath]

		sessions = append(sessions, cs)
	}

	// Sort: pinned first, then by state (working first), then by last activity (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Pinned != sessions[j].Pinned {
			return sessions[i].Pinned
		}
		if sessions[i].State != sessions[j].State {
			return sessions[i].State < sessions[j].State
		}
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	return sessions, nil
}

// inferState determines the activity state from the pane title prefix.
// Braille characters (⠁-⣿) indicate spinner = working.
// ✳ indicates waiting for input.
func inferState(title string) ActivityState {
	for _, r := range title {
		if r == '✳' {
			return StateWaiting
		}
		// Braille pattern characters: U+2800 to U+28FF
		if r >= 0x2800 && r <= 0x28FF {
			return StateWorking
		}
		// Skip leading whitespace
		if unicode.IsSpace(r) {
			continue
		}
		// First non-space, non-indicator character — stop looking
		break
	}

	// If the title contains "Claude Code", it's at least a known session
	if strings.Contains(title, "Claude Code") {
		return StateWaiting
	}
	return StateUnknown
}

// enrichWithHookState reads hook state files and applies live status to a session.
// State files are matched by scanning all files in the state directory and finding
// the most recent one whose session exists in the project's session index.
func enrichWithHookState(cs *ClaudeSession) {
	dir := hook.StateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	homeDir, _ := os.UserHomeDir()
	normalized := strings.ReplaceAll(cs.ProjectPath, "/", "-")
	projectDir := filepath.Join(homeDir, ".claude", "projects", normalized)

	var bestState *hook.State
	var bestTime time.Time

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".json")

		// Check if this session belongs to this project by looking for its JSONL file
		jsonlPath := filepath.Join(projectDir, sessionID+".jsonl")
		if _, err := os.Stat(jsonlPath); err != nil {
			continue
		}

		state, err := hook.ReadState(sessionID)
		if err != nil {
			continue
		}

		t, err := time.Parse(time.RFC3339, state.Timestamp)
		if err != nil {
			continue
		}

		// Only consider state from the last 5 minutes as "live"
		if time.Since(t) > 5*time.Minute {
			continue
		}

		if bestState == nil || t.After(bestTime) {
			bestState = state
			bestTime = t
		}
	}

	if bestState == nil {
		return
	}

	cs.LiveStatus = bestState.Message
	cs.LiveTool = bestState.Tool

	// Override the pane-title-based state with the hook-provided state
	switch bestState.Status {
	case "working":
		cs.State = StateWorking
	case "waiting":
		cs.State = StateWaiting
	case "permission":
		cs.State = StatePermission
	case "done":
		cs.State = StateDone
	}
}
