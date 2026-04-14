package kanban

import (
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
	AgentID    string // Agent Teams agent identifier
	AgentType  string // "teammate", "lead", or empty
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

		// Get summary and branch from session index
		summary, branch, err := session.ReadLatestIndex(p.PanePath)
		if err == nil {
			card.Summary = summary
			card.GitBranch = branch
		}

		// Enrich with live hook state matched by tmux pane ID
		if p.PaneID != "" {
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

	return cards, nil
}
