package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StateDir returns the directory for hook state files.
func StateDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux")
}

// State represents the live status of a Claude session, written by hooks.
type State struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`  // "working", "waiting", "permission"
	Message   string `json:"message"` // short description of current state
	Tool      string `json:"tool"`    // current tool being used (if any)
	Timestamp string `json:"timestamp"`
	PaneID    string `json:"pane_id,omitempty"` // tmux pane ID (e.g. %5)
}

// hookInput is the JSON structure received from Claude Code hooks via stdin.
type hookInput struct {
	SessionID        string          `json:"session_id"`
	TranscriptPath   string          `json:"transcript_path"`
	CWD              string          `json:"cwd"`
	HookEventName    string          `json:"hook_event_name"`
	Prompt           string          `json:"prompt"`             // UserPromptSubmit
	ToolName         string          `json:"tool_name"`          // PreToolUse
	ToolInput        json.RawMessage `json:"tool_input"`         // PreToolUse
	Message          string          `json:"message"`            // Notification
	NotificationType string          `json:"notification_type"`  // Notification
}

type toolInputFields struct {
	Description string `json:"description"`
	Command     string `json:"command"`
	FilePath    string `json:"file_path"`
	Pattern     string `json:"pattern"`
	Query       string `json:"query"`
}

// Handle processes a hook event from stdin and writes a state file.
func Handle(event string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	var input hookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parsing hook input: %w", err)
	}

	if input.SessionID == "" {
		return fmt.Errorf("no session_id in hook input")
	}

	state := State{
		SessionID: input.SessionID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		PaneID:    os.Getenv("TMUX_PANE"),
	}

	switch event {
	case "UserPromptSubmit":
		state.Status = "working"
		state.Message = truncate(input.Prompt, 120)

	case "PreToolUse":
		state.Status = "working"
		state.Tool = input.ToolName
		state.Message = describeToolUse(input.ToolName, input.ToolInput)

	case "Stop":
		state.Status = "done"
		if input.TranscriptPath != "" {
			if msg, err := lastAssistantText(input.TranscriptPath, 32768); err == nil && msg != "" {
				state.Message = msg
			}
		}
		playSound()
		notifyOrchestrator(input.CWD)

	case "Notification":
		switch input.NotificationType {
		case "permission_prompt":
			state.Status = "permission"
		default:
			state.Status = "waiting"
		}
		state.Message = truncate(input.Message, 120)
		playSound()

	default:
		return nil
	}

	return writeState(state)
}

// ReadState reads the state file for a given session ID.
func ReadState(sessionID string) (*State, error) {
	path := filepath.Join(StateDir(), sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ReadStateByPaneID finds the most recent state file for a given tmux pane ID.
func ReadStateByPaneID(paneID string) (*State, error) {
	dir := StateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var best *State
	var bestTime time.Time

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s State
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.PaneID != paneID {
			continue
		}
		t, err := time.Parse(time.RFC3339, s.Timestamp)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestTime) {
			best = &s
			bestTime = t
		}
	}

	if best == nil {
		return nil, os.ErrNotExist
	}
	return best, nil
}

func writeState(s State) error {
	dir := StateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, s.SessionID+".json")
	return os.WriteFile(path, data, 0o644)
}

func describeToolUse(name string, raw json.RawMessage) string {
	var fields toolInputFields
	_ = json.Unmarshal(raw, &fields)

	switch name {
	case "Bash":
		if fields.Description != "" {
			return truncate(fields.Description, 120)
		}
		if fields.Command != "" {
			return "$ " + truncate(fields.Command, 118)
		}
	case "Read":
		if fields.FilePath != "" {
			return "Reading " + shortenFilePath(fields.FilePath)
		}
	case "Edit":
		if fields.FilePath != "" {
			return "Editing " + shortenFilePath(fields.FilePath)
		}
	case "Write":
		if fields.FilePath != "" {
			return "Writing " + shortenFilePath(fields.FilePath)
		}
	case "Glob":
		if fields.Pattern != "" {
			return "Finding " + fields.Pattern
		}
	case "Grep":
		if fields.Pattern != "" {
			return "Searching: " + truncate(fields.Pattern, 109)
		}
	case "WebSearch":
		if fields.Query != "" {
			return "Searching: " + truncate(fields.Query, 109)
		}
	case "Task":
		if fields.Description != "" {
			return "Agent: " + truncate(fields.Description, 113)
		}
	}

	return name
}

func shortenFilePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	return truncate(p, 120)
}

func playSound() {
	sound := os.Getenv("CLAUDE_MUX_SOUND")
	if sound == "0" {
		return
	}
	if sound == "" {
		sound = "/System/Library/Sounds/Funk.aiff"
	}
	cmd := exec.Command("afplay", sound)
	_ = cmd.Start() // fire and forget
}

// notifyOrchestrator checks if this is a subagent and notifies the orchestrator on completion.
func notifyOrchestrator(cwd string) {
	orchPaneFile := filepath.Join(cwd, ".claude-mux", "orchestrator-pane")
	orchPaneBytes, err := os.ReadFile(orchPaneFile)
	if err != nil {
		return
	}
	orchPaneID := strings.TrimSpace(string(orchPaneBytes))

	boardPathBytes, err := os.ReadFile(filepath.Join(cwd, ".claude-mux", "board-path"))
	if err != nil {
		return
	}
	boardPath := strings.TrimSpace(string(boardPathBytes))

	// Load board JSON
	boardData, err := os.ReadFile(boardPath)
	if err != nil {
		return
	}

	var board boardJSON
	if err := json.Unmarshal(boardData, &board); err != nil {
		return
	}

	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		return
	}

	// Find card by pane ID and move to done
	var found *boardCard
	var foundCol string
	for col, cards := range board.Columns {
		for i := range cards {
			if cards[i].PaneID == paneID {
				found = &cards[i]
				foundCol = col
				break
			}
		}
	}

	if found == nil {
		return
	}

	// Remove from current column
	var remaining []boardCard
	for _, c := range board.Columns[foundCol] {
		if c.PaneID != paneID {
			remaining = append(remaining, c)
		}
	}
	board.Columns[foundCol] = remaining

	// Add to done
	board.Columns["done"] = append(board.Columns["done"], *found)

	// Save board
	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		return
	}
	tmpPath := boardPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return
	}
	os.Rename(tmpPath, boardPath)

	// Send notification to orchestrator pane
	msg := fmt.Sprintf("Agent %s completed on branch %s", found.TaskID, found.Branch)
	exec.Command("tmux", "send-keys", "-t", orchPaneID, msg, "Enter").Run()
}

// boardJSON is a minimal representation of the kanban board for hook use (avoids import cycle).
type boardJSON struct {
	SwarmID            string                  `json:"swarm_id"`
	OrchestratorPaneID string                  `json:"orchestrator_pane_id"`
	Columns            map[string][]boardCard  `json:"columns"`
}

type boardCard struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	PaneID      string `json:"pane_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
}

func truncate(s string, max int) string {
	// Take first line only
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
