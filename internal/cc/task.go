package cc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TaskState represents a task session within the CC.
type TaskState struct {
	ID        string    `json:"id"`
	Task      string    `json:"task"`
	RepoRoot  string    `json:"repo_root"`
	WindowID  string    `json:"window_id"`
	PaneID    string    `json:"pane_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TasksState holds all active task sessions.
type TasksState struct {
	Tasks []TaskState `json:"tasks"`
}

func tasksFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "cc-tasks.json")
}

func loadTasks() (*TasksState, error) {
	data, err := os.ReadFile(tasksFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &TasksState{}, nil
		}
		return nil, err
	}
	var ts TasksState
	if err := json.Unmarshal(data, &ts); err != nil {
		return &TasksState{}, nil
	}
	return &ts, nil
}

func saveTasks(ts *TasksState) error {
	path := tasksFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// generateTaskID creates a short unique ID for a task session.
func generateTaskID() string {
	return fmt.Sprintf("cc-%d", time.Now().UnixMilli()%100000)
}

// NewTask creates a new task session as a window in the CC's tmux socket.
// It launches Claude with the CC system prompt in the new window.
// If the CC is not running, it starts it first.
func NewTask(task string, repoRoot string) (*TaskState, error) {
	// Ensure CC session exists on the separate socket
	if _, err := EnsureRunning(repoRoot); err != nil {
		return nil, fmt.Errorf("ensuring CC is running: %w", err)
	}

	taskID := generateTaskID()

	// Write system prompt to a task-specific file
	home, _ := os.UserHomeDir()
	promptDir := filepath.Join(home, ".cache", "claude-mux")
	os.MkdirAll(promptDir, 0o755)
	promptFile := filepath.Join(promptDir, fmt.Sprintf("cc-system-prompt-%s.txt", taskID))

	// Build a task-scoped system prompt that includes the task description
	taskPrompt := SystemPrompt()
	if task != "" {
		taskPrompt += fmt.Sprintf("\n\n## Current Task\n\nYou have been assigned the following task:\n\n%s\n\nFocus on this task. Use claude-mux spawn/swarm to delegate implementation work to subagents.", task)
	}

	if err := os.WriteFile(promptFile, []byte(taskPrompt), 0o644); err != nil {
		return nil, fmt.Errorf("write task prompt: %w", err)
	}

	// Create a new window in the CC session
	windowName := taskID
	newWindowArgs := []string{
		"-L", socketName,
		"new-window",
		"-d",
		"-t", sessionName + ":",
		"-n", windowName,
		"-c", repoRoot,
		"-P", "-F", "#{window_id}:#{pane_id}",
	}
	out, err := runTmuxOutput(newWindowArgs...)
	if err != nil {
		return nil, fmt.Errorf("create task window: %w", err)
	}

	parts := strings.SplitN(out, ":", 2)
	windowID := parts[0]
	paneID := out
	if len(parts) == 2 {
		paneID = parts[1]
	}

	// Set environment variables in the new pane
	mainTmux := os.Getenv("TMUX")
	if mainTmux != "" {
		_ = runTmux("-L", socketName, "send-keys", "-t", paneID, fmt.Sprintf("export TMUX='%s'", mainTmux), "Enter")
	}
	_ = runTmux("-L", socketName, "send-keys", "-t", paneID, "export CLAUDE_MUX_CC=1", "Enter")

	// Launch Claude with the task-scoped system prompt
	claudeCmd := fmt.Sprintf(`claude --append-system-prompt "$(cat %s)"`, promptFile)
	if task != "" {
		// Pass the task as the initial message
		claudeCmd += fmt.Sprintf(` "%s"`, strings.ReplaceAll(task, `"`, `\"`))
	}
	if err := runTmux("-L", socketName, "send-keys", "-t", paneID, claudeCmd, "Enter"); err != nil {
		return nil, fmt.Errorf("launch claude in task window: %w", err)
	}

	state := &TaskState{
		ID:        taskID,
		Task:      task,
		RepoRoot:  repoRoot,
		WindowID:  windowID,
		PaneID:    paneID,
		CreatedAt: time.Now(),
	}

	// Save to tasks list
	tasks, _ := loadTasks()
	tasks.Tasks = append(tasks.Tasks, *state)
	if err := saveTasks(tasks); err != nil {
		return nil, fmt.Errorf("save tasks: %w", err)
	}

	return state, nil
}

// ListTasks returns all task sessions, pruning any whose windows no longer exist.
func ListTasks() ([]TaskState, error) {
	tasks, err := loadTasks()
	if err != nil {
		return nil, err
	}

	// Prune dead windows
	var alive []TaskState
	for _, t := range tasks.Tasks {
		err := runTmux("-L", socketName, "has-session", "-t", sessionName)
		if err != nil {
			continue
		}
		// Check if window still exists by trying to select it
		err = runTmux("-L", socketName, "select-window", "-t", fmt.Sprintf("%s:%s", sessionName, t.WindowID))
		if err == nil {
			alive = append(alive, t)
		}
	}

	if len(alive) != len(tasks.Tasks) {
		tasks.Tasks = alive
		saveTasks(tasks)
	}

	return alive, nil
}

// FocusTask switches to a specific task's window in the CC socket and opens it.
func FocusTask(taskID string, width, height string) error {
	tasks, err := loadTasks()
	if err != nil {
		return err
	}

	for _, t := range tasks.Tasks {
		if t.ID == taskID {
			// Select the task's window
			_ = runTmux("-L", socketName, "select-window", "-t", fmt.Sprintf("%s:%s", sessionName, t.WindowID))

			// Open the CC session (will show the selected window)
			return Open(width, height)
		}
	}

	return fmt.Errorf("task %q not found", taskID)
}
