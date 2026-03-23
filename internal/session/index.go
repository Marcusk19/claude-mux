package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
	FirstPrompt  string `json:"firstPrompt"`
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

// findMostRecentSession finds the most recently modified JSONL file on disk
// for the given project path, then looks up its metadata in sessions-index.json.
// Falls back to the most recent index entry if no JSONL files are found.
func findMostRecentSession(panePath string) (*indexEntry, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	normalized := normalizePath(panePath)
	projectDir := filepath.Join(homeDir, ".claude", "projects", normalized)

	// Build a map of index entries keyed by session ID.
	indexMap := make(map[string]*indexEntry)
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	if data, err := os.ReadFile(indexPath); err == nil {
		var idx sessionsIndex
		if err := json.Unmarshal(data, &idx); err == nil {
			for i := range idx.Entries {
				e := idx.Entries[i]
				indexMap[e.SessionID] = &e
			}
		}
	}

	// Scan for JSONL files and pick the most recently modified one.
	matches, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	type jsonlFile struct {
		path    string
		modTime time.Time
	}
	var files []jsonlFile
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		files = append(files, jsonlFile{path: m, modTime: info.ModTime()})
	}

	// Sort by modification time, most recent first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if len(files) > 0 {
		best := files[0]
		sessionID := strings.TrimSuffix(filepath.Base(best.path), ".jsonl")

		if entry, ok := indexMap[sessionID]; ok {
			// Ensure FullPath is set so JSONL can be read for LastActivity.
			if entry.FullPath == "" {
				entry.FullPath = best.path
			}
			return entry, nil
		}

		// Session not in index — return a minimal entry so the caller
		// can still read the JSONL for timestamps and first prompt.
		return &indexEntry{
			SessionID: sessionID,
			FullPath:  best.path,
		}, nil
	}

	// No JSONL files found — fall back to the most recent index entry.
	var best *indexEntry
	var bestTime time.Time
	for _, e := range indexMap {
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

// ReadLatestIndex returns the summary and git branch from the most recent session
// for the given pane path.
func ReadLatestIndex(panePath string) (summary string, branch string, err error) {
	entry, err := findMostRecentSession(panePath)
	if err != nil {
		return "", "", err
	}
	return entry.Summary, entry.GitBranch, nil
}
