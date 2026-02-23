package pin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

func pinsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "pins.json")
}

// Load returns the list of pinned project paths.
func Load() []string {
	data, err := os.ReadFile(pinsPath())
	if err != nil {
		return nil
	}
	var pins []string
	if err := json.Unmarshal(data, &pins); err != nil {
		return nil
	}
	return pins
}

// Save writes the list of pinned project paths to disk.
func Save(pins []string) error {
	p := pinsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(pins)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Toggle adds or removes a project path from the pin list.
func Toggle(path string) {
	pins := Load()
	if i := slices.Index(pins, path); i >= 0 {
		pins = slices.Delete(pins, i, i+1)
	} else {
		pins = append(pins, path)
	}
	Save(pins)
}

// IsPinned returns true if the given project path is pinned.
func IsPinned(path string) bool {
	return slices.Contains(Load(), path)
}
