package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type sessionsIndex struct {
	Version int          `json:"version"`
	Entries []indexEntry `json:"entries"`
}

type indexEntry struct {
	SessionID    string `json:"sessionId"`
	FullPath     string `json:"fullPath"`
	Summary      string `json:"summary"`
	GitBranch    string `json:"gitBranch"`
	MessageCount int    `json:"messageCount"`
	Modified     string `json:"modified"`
	ProjectPath  string `json:"projectPath"`
}

// normalizePath converts a filesystem path to the Claude projects directory name.
// e.g., "/Users/mkok/workspace/project" -> "-Users-mkok-workspace-project"
func normalizePath(p string) string {
	return strings.ReplaceAll(p, "/", "-")
}

// findMostRecentSession reads sessions-index.json from the Claude projects
// directory for the given path and returns the most recently modified entry.
func findMostRecentSession(panePath string) (*indexEntry, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	normalized := normalizePath(panePath)
	indexPath := filepath.Join(homeDir, ".claude", "projects", normalized, "sessions-index.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var idx sessionsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	if len(idx.Entries) == 0 {
		return nil, os.ErrNotExist
	}

	var best *indexEntry
	var bestTime time.Time

	for i := range idx.Entries {
		e := &idx.Entries[i]
		t, err := time.Parse(time.RFC3339Nano, e.Modified)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestTime) {
			best = e
			bestTime = t
		}
	}

	if best == nil {
		return nil, os.ErrNotExist
	}
	return best, nil
}
