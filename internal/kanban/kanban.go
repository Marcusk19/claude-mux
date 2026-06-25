package kanban

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcusk19/claude-mux/internal/hook"
	"github.com/Marcusk19/claude-mux/internal/session"
	"github.com/Marcusk19/claude-mux/internal/tmux"
)

// PaneCard represents a single Claude agent pane in the kanban view.
type PaneCard struct {
	Pane       tmux.PaneInfo
	State      session.ActivityState
	GitBranch  string
	LiveStatus string
	LiveTool   string
	Summary    string
	AgentID         string // Agent Teams agent identifier
	AgentType       string // "teammate", "lead", or empty
	TaskDescription string // Orchestrator task description
}

// DiscoverKanban finds Claude panes in a specific tmux window and returns cards for them.
func DiscoverKanban(sessionName, windowIndex string) ([]PaneCard, error) {
	panes, err := tmux.ListWindowPanes(sessionName, windowIndex)
	if err != nil {
		return nil, err
	}

	var cards []PaneCard
	for _, p := range panes {
		if !tmux.IsClaudePane(p) {
			continue
		}

		card := PaneCard{
			Pane:  p,
			State: session.InferState(p.PaneTitle),
		}

		// Sandboxed panes (openshell) have limited status visibility
		sandboxed := tmux.IsOpenShellPane(p)
		if sandboxed {
			card.State = session.StateSandboxed
		}

		// Get summary and branch from session index
		summary, branch, err := session.ReadLatestIndex(p.PanePath)
		if err == nil {
			card.Summary = summary
			card.GitBranch = branch
		}

		// Enrich with live hook state matched by tmux pane ID
		// (not available for sandboxed sessions)
		if p.PaneID != "" && !sandboxed {
			hookState, err := hook.ReadStateByPaneID(p.PaneID)
			if err == nil && hookState != nil {
				t, err := time.Parse(time.RFC3339, hookState.Timestamp)
				if err == nil && time.Since(t) < 5*time.Minute {
					card.LiveStatus = hookState.Message
					card.LiveTool = hookState.Tool
					card.AgentID = hookState.AgentID
					card.AgentType = hookState.AgentType

					switch hookState.Status {
					case "working":
						card.State = session.StateWorking
					case "waiting":
						card.State = session.StateWaiting
					case "permission":
						card.State = session.StatePermission
					case "done":
						card.State = session.StateDone
					}
				}
			}
		}

		cards = append(cards, card)
	}

	// Enrich with orchestrator task descriptions
	tasksByPane := LoadTasksByPane()
	for i := range cards {
		if desc, ok := tasksByPane[cards[i].Pane.PaneID]; ok {
			cards[i].TaskDescription = desc
		}
	}

	return cards, nil
}

// LoadTasksByPane loads orchestrator state files and returns a map from pane ID to task description.
func LoadTasksByPane() map[string]string {
	result := make(map[string]string)
	home, _ := os.UserHomeDir()
	orchDir := filepath.Join(home, ".cache", "claude-mux", "orchestrator")

	orchEntries, err := os.ReadDir(orchDir)
	if err != nil {
		return result
	}

	for _, orchEntry := range orchEntries {
		if !orchEntry.IsDir() {
			continue
		}
		taskDir := filepath.Join(orchDir, orchEntry.Name())
		taskEntries, err := os.ReadDir(taskDir)
		if err != nil {
			continue
		}
		for _, taskEntry := range taskEntries {
			if taskEntry.IsDir() || !strings.HasSuffix(taskEntry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(taskDir, taskEntry.Name()))
			if err != nil {
				continue
			}
			var state struct {
				PaneID string `json:"pane_id"`
				Task   string `json:"task"`
			}
			if err := json.Unmarshal(data, &state); err != nil {
				continue
			}
			if state.PaneID != "" && state.Task != "" {
				result[state.PaneID] = state.Task
			}
		}
	}

	return result
}
