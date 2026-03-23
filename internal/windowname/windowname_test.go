package windowname

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupKey(t *testing.T) {
	tests := []struct {
		session, window, want string
	}{
		{"main", "0", "main:0"},
		{"dev", "3", "dev:3"},
		{"", "", ":"},
	}
	for _, tt := range tests {
		got := GroupKey(tt.session, tt.window)
		if got != tt.want {
			t.Errorf("GroupKey(%q, %q) = %q, want %q", tt.session, tt.window, got, tt.want)
		}
	}
}

func TestDeriveGroupName(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{"empty", nil, ""},
		{"single", []string{"/home/user/project"}, "project"},
		{"majority wins", []string{"/a/foo", "/b/foo", "/c/bar"}, "foo"},
		{"tie broken alphabetically", []string{"/a/beta", "/b/alpha"}, "alpha"},
		{"all same", []string{"/x/same", "/y/same"}, "same"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveGroupName(tt.paths)
			if got != tt.want {
				t.Errorf("DeriveGroupName(%v) = %q, want %q", tt.paths, got, tt.want)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	custom := map[string]string{"main:0": "my-window"}

	// Custom name takes precedence
	got := DisplayName("main:0", custom, []string{"/a/foo", "/b/bar"})
	if got != "my-window" {
		t.Errorf("DisplayName with custom = %q, want %q", got, "my-window")
	}

	// Falls back to derived name
	got = DisplayName("main:1", custom, []string{"/a/foo", "/b/foo"})
	if got != "foo" {
		t.Errorf("DisplayName without custom = %q, want %q", got, "foo")
	}

	// No custom, no paths
	got = DisplayName("main:2", custom, nil)
	if got != "" {
		t.Errorf("DisplayName empty = %q, want %q", got, "")
	}
}

func TestLoadSaveRoundtrip(t *testing.T) {
	// Use a temp dir to avoid touching real cache
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Load from non-existent file returns empty map
	names := Load()
	if len(names) != 0 {
		t.Fatalf("Load() from empty = %v, want empty map", names)
	}

	// Save and reload
	names["main:0"] = "dev-window"
	names["work:1"] = "test-window"
	if err := Save(names); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, ".cache", "claude-mux", "window-names.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	loaded := Load()
	if len(loaded) != 2 {
		t.Fatalf("Load() got %d entries, want 2", len(loaded))
	}
	if loaded["main:0"] != "dev-window" {
		t.Errorf("loaded[main:0] = %q, want %q", loaded["main:0"], "dev-window")
	}
	if loaded["work:1"] != "test-window" {
		t.Errorf("loaded[work:1] = %q, want %q", loaded["work:1"], "test-window")
	}
}
