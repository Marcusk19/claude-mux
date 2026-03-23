package session

import (
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
			State:       InferState(p.PaneTitle),
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

				cs.CurrentActivity = readLastAssistantText(entry.FullPath)

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

	// Sort: pinned first, then by last activity (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Pinned != sessions[j].Pinned {
			return sessions[i].Pinned
		}
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	return sessions, nil
}

// InferState determines the activity state from the pane title prefix.
// Braille characters (⠁-⣿) indicate spinner = working.
// ✳ indicates waiting for input.
func InferState(title string) ActivityState {
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
// Matches by tmux pane ID to correctly distinguish multiple sessions in the same project.
func enrichWithHookState(cs *ClaudeSession) {
	if cs.Pane.PaneID == "" {
		return
	}

	state, err := hook.ReadStateByPaneID(cs.Pane.PaneID)
	if err != nil {
		return
	}

	t, err := time.Parse(time.RFC3339, state.Timestamp)
	if err != nil || time.Since(t) > 5*time.Minute {
		return
	}

	cs.LiveStatus = state.Message
	cs.LiveTool = state.Tool

	// Override the pane-title-based state with the hook-provided state
	switch state.Status {
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
