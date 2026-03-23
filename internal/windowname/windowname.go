package windowname

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// GroupKey returns a unique key for a tmux window: "session_name:window_index".
func GroupKey(sessionName, windowIndex string) string {
	return fmt.Sprintf("%s:%s", sessionName, windowIndex)
}

// DeriveGroupName returns the most common filepath.Base() from the given paths.
// Ties are broken alphabetically (earliest name wins).
func DeriveGroupName(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	counts := make(map[string]int)
	for _, p := range paths {
		base := filepath.Base(p)
		counts[base]++
	}

	var best string
	bestCount := 0
	for name, count := range counts {
		if count > bestCount || (count == bestCount && (best == "" || name < best)) {
			best = name
			bestCount = count
		}
	}
	return best
}

func cacheFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "window-names.json")
}

// Load reads custom window names from ~/.cache/claude-mux/window-names.json.
func Load() map[string]string {
	data, err := os.ReadFile(cacheFilePath())
	if err != nil {
		return make(map[string]string)
	}
	var names map[string]string
	if err := json.Unmarshal(data, &names); err != nil {
		return make(map[string]string)
	}
	return names
}

// Save writes custom window names to ~/.cache/claude-mux/window-names.json.
func Save(names map[string]string) error {
	path := cacheFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	data, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// DisplayName returns the custom name for a window if set, otherwise derives one from paths.
func DisplayName(key string, customNames map[string]string, paths []string) string {
	if name, ok := customNames[key]; ok {
		return name
	}
	return DeriveGroupName(paths)
}
