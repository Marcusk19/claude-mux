package session

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mkok/claude-mux/internal/tmux"
)

// DiscoverSessions finds all Claude Code panes and enriches them with session data.
func DiscoverSessions() ([]ClaudeSession, error) {
	panes, err := tmux.ListPanes()
	if err != nil {
		return nil, err
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
			}
		}

		// Fall back to modified time if no JSONL activity
		if cs.LastActivity.IsZero() && !cs.Modified.IsZero() {
			cs.LastActivity = cs.Modified
		}

		sessions = append(sessions, cs)
	}

	// Sort: working sessions first, then by last activity (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
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
