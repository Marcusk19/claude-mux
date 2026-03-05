package session

import (
	"time"

	"github.com/mkok/claude-mux/internal/tmux"
)

// ActivityState represents the current state of a Claude session.
type ActivityState int

const (
	StateWorking    ActivityState = iota
	StateWaiting
	StatePermission
	StateDone
	StateUnknown
)

func (s ActivityState) String() string {
	switch s {
	case StateWorking:
		return "Working"
	case StateWaiting:
		return "Waiting"
	case StatePermission:
		return "Permission"
	case StateDone:
		return "Done"
	default:
		return "Unknown"
	}
}

func (s ActivityState) Emoji() string {
	switch s {
	case StateWorking:
		return "⏳"
	case StateWaiting:
		return "🔒"
	case StatePermission:
		return "🔐"
	case StateDone:
		return "😊"
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
	InitialPrompt   string // truncated first user message in the session
	CurrentActivity string // last assistant message text, truncated
	LiveStatus      string // live status message from hooks (what Claude is doing/asking)
	LiveTool      string // current tool being used
	Pinned        bool   // user-pinned to top of list
}
