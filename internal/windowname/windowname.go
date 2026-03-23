package windowname

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// stateFile returns the path to the window names JSON file.
func stateFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "window-names.json")
}

// GroupKey returns a unique key for grouping sessions by tmux session + window.
func GroupKey(sessionName, windowIndex string) string {
	return sessionName + ":" + windowIndex
}

// DeriveGroupName returns the most common filepath.Base() from a list of paths.
func DeriveGroupName(paths []string) string {
	if len(paths) == 0 {
		return "unknown"
	}
	counts := make(map[string]int)
	for _, p := range paths {
		base := filepath.Base(p)
		counts[base]++
	}
	best := ""
	bestCount := 0
	for name, count := range counts {
		if count > bestCount || (count == bestCount && name < best) {
			best = name
			bestCount = count
		}
	}
	if best == "" {
		return "unknown"
	}
	return best
}

// DisplayName returns a custom name if set, otherwise the derived name.
func DisplayName(groupKey string, paths []string) string {
	names := Load()
	if custom, ok := names[groupKey]; ok && custom != "" {
		return custom
	}
	return DeriveGroupName(paths)
}

// Load reads custom window names from the state file.
func Load() map[string]string {
	data, err := os.ReadFile(stateFile())
	if err != nil {
		return make(map[string]string)
	}
	var names map[string]string
	if err := json.Unmarshal(data, &names); err != nil {
		return make(map[string]string)
	}
	return names
}

// Save writes custom window names to the state file.
func Save(names map[string]string) error {
	path := stateFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// FormatGroupHeader returns a formatted header string for a group.
func FormatGroupHeader(name string, count int, collapsed bool) string {
	if collapsed {
		return "▶ " + name + " (collapsed)"
	}
	ruler := strings.Repeat("─", 4)
	return "▼ " + name + " (" + itoa(count) + " sessions) " + ruler
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
