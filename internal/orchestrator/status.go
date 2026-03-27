package orchestrator

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Marcusk19/claude-mux/internal/hook"
	"github.com/Marcusk19/claude-mux/internal/tmux"
)

// StatusResult holds the resolved status of a subagent.
type StatusResult struct {
	SubagentState
	LiveStatus string // from hook state enrichment
	LiveTool   string
}

// Status returns the current state of all subagents for the current orchestrator.
func Status() ([]StatusResult, error) {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return nil, err
	}

	states, err := listStates(orchID)
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, nil
	}

	// Get active pane IDs
	activePanes := make(map[string]bool)
	panes, err := tmux.ListPanes()
	if err == nil {
		for _, p := range panes {
			activePanes[p.PaneID] = true
		}
	}

	var results []StatusResult
	for _, s := range states {
		r := StatusResult{SubagentState: s}

		if s.Status == "running" {
			if !activePanes[s.PaneID] {
				// Pane is gone — detect completion
				s.Status = detectCompletion(s)
				now := time.Now()
				s.CompletedAt = &now
				writeState(s)
				r.SubagentState = s
			} else {
				// Enrich with hook state
				enrichWithHookState(&r)
			}
		}

		results = append(results, r)
	}
	return results, nil
}

// FormatStatus produces a human-readable status table.
func FormatStatus(results []StatusResult) string {
	if len(results) == 0 {
		return "No subagents found."
	}

	var b strings.Builder
	for _, r := range results {
		age := time.Since(r.CreatedAt).Truncate(time.Second)
		status := r.Status
		if r.LiveStatus != "" {
			status = r.LiveStatus
		}

		fmt.Fprintf(&b, "[%s] %s  (%s, %s ago)\n", r.TaskID, status, r.BranchName, age)

		task := r.Task
		if len(task) > 80 {
			task = task[:79] + "…"
		}
		fmt.Fprintf(&b, "  Task: %s\n", task)

		if r.LiveTool != "" {
			fmt.Fprintf(&b, "  Tool: %s\n", r.LiveTool)
		}
		fmt.Fprintf(&b, "  Pane: %s  Worktree: %s\n", r.PaneID, r.WorktreePath)
		b.WriteString("\n")
	}
	return b.String()
}

// detectCompletion checks if a finished subagent completed successfully.
func detectCompletion(s SubagentState) string {
	// Check if the branch has commits beyond the base
	out, err := exec.Command(
		"git", "-C", s.RepoRoot,
		"log", "--oneline", "HEAD.."+s.BranchName,
	).Output()
	if err != nil {
		return "failed"
	}
	if strings.TrimSpace(string(out)) != "" {
		return "completed"
	}
	return "failed"
}

// enrichWithHookState adds live status from hook state files.
func enrichWithHookState(r *StatusResult) {
	// Hook state files are at ~/.cache/claude-mux/<session-id>.json
	// We need to find a hook state that matches this subagent's pane.
	// The hook session ID is in the JSONL filename, not directly tied to our task ID.
	// For now, try to find a recent hook state by scanning.
	state, err := hook.ReadStateByPaneID(r.PaneID)
	if err != nil {
		return
	}
	t, err := time.Parse(time.RFC3339, state.Timestamp)
	if err != nil || time.Since(t) > 5*time.Minute {
		return
	}
	r.LiveStatus = state.Status
	r.LiveTool = state.Tool
}
