package tmux

import (
	"strings"
	"testing"
)

func TestParsePaneLines(t *testing.T) {
	// Build a line using the actual delimiter
	makeLine := func(fields ...string) string {
		return strings.Join(fields, delimiter)
	}

	t.Run("single pane", func(t *testing.T) {
		raw := makeLine("quay", "1", "0", "✳ Claude Code", "2.1.81", "/Users/test/workspace", "%42", "zsh")
		panes := parsePaneLines(raw)
		if len(panes) != 1 {
			t.Fatalf("expected 1 pane, got %d", len(panes))
		}
		p := panes[0]
		if p.SessionName != "quay" {
			t.Errorf("SessionName = %q, want %q", p.SessionName, "quay")
		}
		if p.WindowIndex != "1" {
			t.Errorf("WindowIndex = %q, want %q", p.WindowIndex, "1")
		}
		if p.PaneIndex != "0" {
			t.Errorf("PaneIndex = %q, want %q", p.PaneIndex, "0")
		}
		if p.PaneTitle != "✳ Claude Code" {
			t.Errorf("PaneTitle = %q, want %q", p.PaneTitle, "✳ Claude Code")
		}
		if p.PaneCommand != "2.1.81" {
			t.Errorf("PaneCommand = %q, want %q", p.PaneCommand, "2.1.81")
		}
		if p.PanePath != "/Users/test/workspace" {
			t.Errorf("PanePath = %q, want %q", p.PanePath, "/Users/test/workspace")
		}
		if p.PaneID != "%42" {
			t.Errorf("PaneID = %q, want %q", p.PaneID, "%42")
		}
		if p.WindowName != "zsh" {
			t.Errorf("WindowName = %q, want %q", p.WindowName, "zsh")
		}
	})

	t.Run("multiple panes", func(t *testing.T) {
		raw := makeLine("quay", "1", "0", "✳ Claude Code", "2.1.81", "/path/a", "%1", "zsh") + "\n" +
			makeLine("quay", "1", "1", "mkok-mac", "zsh", "/path/b", "%2", "zsh")
		panes := parsePaneLines(raw)
		if len(panes) != 2 {
			t.Fatalf("expected 2 panes, got %d", len(panes))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		panes := parsePaneLines("")
		if len(panes) != 0 {
			t.Fatalf("expected 0 panes, got %d", len(panes))
		}
	})

	t.Run("malformed line skipped", func(t *testing.T) {
		raw := "not-enough-fields\n" +
			makeLine("quay", "1", "0", "Claude Code", "2.1.81", "/path", "%1", "zsh")
		panes := parsePaneLines(raw)
		if len(panes) != 1 {
			t.Fatalf("expected 1 pane (skipping malformed), got %d", len(panes))
		}
	})

	t.Run("blank lines skipped", func(t *testing.T) {
		raw := "\n" + makeLine("quay", "1", "0", "title", "cmd", "/path", "%1", "win") + "\n\n"
		panes := parsePaneLines(raw)
		if len(panes) != 1 {
			t.Fatalf("expected 1 pane, got %d", len(panes))
		}
	})
}

func TestParseCurrentWindow(t *testing.T) {
	t.Run("valid tab-delimited output", func(t *testing.T) {
		session, window, err := parseCurrentWindow("quay\t6\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session != "quay" {
			t.Errorf("session = %q, want %q", session, "quay")
		}
		if window != "6" {
			t.Errorf("window = %q, want %q", window, "6")
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		session, window, err := parseCurrentWindow("main\t0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session != "main" || window != "0" {
			t.Errorf("got %q:%q, want main:0", session, window)
		}
	})

	t.Run("old %%DELIM%% format fails", func(t *testing.T) {
		// This is what display-message -p would produce with %%DELIM%%:
		// %% becomes %, so %%DELIM%% becomes %DELIM%
		_, _, err := parseCurrentWindow("quay%DELIM%6")
		if err == nil {
			t.Fatal("expected error for old DELIM-style output, got nil")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, _, err := parseCurrentWindow("")
		if err == nil {
			t.Fatal("expected error for empty input")
		}
	})

	t.Run("too many fields", func(t *testing.T) {
		_, _, err := parseCurrentWindow("a\tb\tc")
		if err == nil {
			t.Fatal("expected error for 3 tab-separated fields")
		}
	})
}

func TestIsClaudePane(t *testing.T) {
	tests := []struct {
		name string
		pane PaneInfo
		want bool
	}{
		{
			name: "semver command",
			pane: PaneInfo{PaneCommand: "2.1.81", PaneTitle: "something"},
			want: true,
		},
		{
			name: "semver command with Claude Code title",
			pane: PaneInfo{PaneCommand: "2.1.85", PaneTitle: "✳ Claude Code"},
			want: true,
		},
		{
			name: "Claude Code title with non-shell command",
			pane: PaneInfo{PaneCommand: "node", PaneTitle: "Claude Code"},
			want: true,
		},
		{
			name: "Claude Code title with braille spinner",
			pane: PaneInfo{PaneCommand: "node", PaneTitle: "⠐ Claude Code"},
			want: true,
		},
		{
			name: "Claude Code title but shell command - exited session",
			pane: PaneInfo{PaneCommand: "zsh", PaneTitle: "Claude Code"},
			want: false,
		},
		{
			name: "Claude Code title but bash - exited session",
			pane: PaneInfo{PaneCommand: "bash", PaneTitle: "Claude Code"},
			want: false,
		},
		{
			name: "plain shell",
			pane: PaneInfo{PaneCommand: "zsh", PaneTitle: "mkok-mac"},
			want: false,
		},
		{
			name: "other process",
			pane: PaneInfo{PaneCommand: "nvim", PaneTitle: "file.go - Nvim"},
			want: false,
		},
		{
			name: "not semver - has extra suffix",
			pane: PaneInfo{PaneCommand: "2.1.81-beta", PaneTitle: "something"},
			want: false,
		},
		{
			name: "not semver - only two parts",
			pane: PaneInfo{PaneCommand: "2.1", PaneTitle: "something"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsClaudePane(tt.pane)
			if got != tt.want {
				t.Errorf("IsClaudePane(%+v) = %v, want %v", tt.pane, got, tt.want)
			}
		})
	}
}

func TestIsShell(t *testing.T) {
	shells := []string{"zsh", "bash", "sh", "fish", "dash", "ksh", "tcsh", "csh"}
	for _, s := range shells {
		if !isShell(s) {
			t.Errorf("isShell(%q) = false, want true", s)
		}
	}

	nonShells := []string{"node", "nvim", "2.1.81", "python", "claude-mux", ""}
	for _, s := range nonShells {
		if isShell(s) {
			t.Errorf("isShell(%q) = true, want false", s)
		}
	}
}
