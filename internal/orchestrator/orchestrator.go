package orchestrator

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SubagentState tracks a spawned subagent's lifecycle.
type SubagentState struct {
	TaskID         string     `json:"task_id"`
	OrchestratorID string     `json:"orchestrator_id"`
	Task           string     `json:"task"`
	WorktreePath   string     `json:"worktree_path"`
	BranchName     string     `json:"branch_name"`
	RepoRoot       string     `json:"repo_root"`
	PaneID         string     `json:"pane_id"`
	Status         string     `json:"status"` // running, completed, failed
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Sandboxed      bool       `json:"sandboxed,omitempty"`
	ContainerName  string     `json:"container_name,omitempty"`
}

// stateDir returns the directory for a given orchestrator's state files.
func stateDir(orchID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "orchestrator", orchID)
}

// writeState persists a subagent state to disk.
func writeState(s SubagentState) error {
	dir := stateDir(s.OrchestratorID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.TaskID+".json"), data, 0o644)
}

// readState reads a single subagent state file.
func readState(orchID, taskID string) (*SubagentState, error) {
	data, err := os.ReadFile(filepath.Join(stateDir(orchID), taskID+".json"))
	if err != nil {
		return nil, err
	}
	var s SubagentState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// listStates returns all subagent states for an orchestrator.
func listStates(orchID string) ([]SubagentState, error) {
	dir := stateDir(orchID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var states []SubagentState
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s SubagentState
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		states = append(states, s)
	}
	return states, nil
}

// deleteState removes a subagent state file.
func deleteState(orchID, taskID string) error {
	return os.Remove(filepath.Join(stateDir(orchID), taskID+".json"))
}

// generateTaskID creates a timestamp + random hex ID matching the worktree pattern.
func generateTaskID() string {
	ts := time.Now().Format("20060102-150405")
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("%s-%s", ts, hex.EncodeToString(b))
}

// resolveOrchestratorID determines the orchestrator identity.
// Priority: CLAUDE_SESSION_ID env var > .claude-mux/orchestrator-id file > generate new.
func resolveOrchestratorID() (string, error) {
	if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
		return id, nil
	}

	// Try reading from repo root
	idFile := findOrchestratorIDFile()
	if idFile != "" {
		data, err := os.ReadFile(idFile)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				return id, nil
			}
		}
	}

	// Generate new ID and persist it
	id := generateTaskID()
	if idFile == "" {
		// Not in a git repo, use current dir
		idFile = filepath.Join(".claude-mux", "orchestrator-id")
	}
	if err := os.MkdirAll(filepath.Dir(idFile), 0o755); err != nil {
		return id, nil // return ID even if we can't persist
	}
	os.WriteFile(idFile, []byte(id+"\n"), 0o644)
	return id, nil
}

// findOrchestratorIDFile looks for .claude-mux/orchestrator-id relative to git root.
func findOrchestratorIDFile() string {
	root, err := gitRepoRoot(".")
	if err != nil {
		return ""
	}
	return filepath.Join(root, ".claude-mux", "orchestrator-id")
}
