package session

import (
	"time"

	"github.com/mkok/claude-mux/internal/tmux"
)

// ActivityState represents the current state of a Claude session.
type ActivityState int

const (
	StateWorking ActivityState = iota
	StateWaiting
	StateUnknown
)

func (s ActivityState) String() string {
	switch s {
	case StateWorking:
		return "Working"
	case StateWaiting:
		return "Waiting"
	default:
		return "Unknown"
	}
}

func (s ActivityState) Emoji() string {
	switch s {
	case StateWorking:
		return "⚡"
	case StateWaiting:
		return "⏳"
	default:
		return "❓"
	}
}

// ClaudeSession combines tmux pane info with Claude session metadata.
type ClaudeSession struct {
	Pane         tmux.PaneInfo
	Summary      string
	GitBranch    string
	MessageCount int
	Modified     time.Time
	LastActivity time.Time
	State        ActivityState
	ProjectPath  string
}
