package kanban

import (
	"time"

	"github.com/mkok/claude-mux/internal/hook"
	"github.com/mkok/claude-mux/internal/session"
	"github.com/mkok/claude-mux/internal/tmux"
)

// PaneCard represents a single Claude agent pane in the kanban view.
type PaneCard struct {
	Pane       tmux.PaneInfo
	State      session.ActivityState
	GitBranch  string
	LiveStatus string
	LiveTool   string
	Summary    string
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

		// Enrich with live hook state
		hookState, err := hook.ReadStateByPath(p.PanePath)
		if err == nil && hookState != nil {
			t, err := time.Parse(time.RFC3339, hookState.Timestamp)
			if err == nil && time.Since(t) < 5*time.Minute {
				card.LiveStatus = hookState.Message
				card.LiveTool = hookState.Tool

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

		cards = append(cards, card)
	}

	return cards, nil
}
