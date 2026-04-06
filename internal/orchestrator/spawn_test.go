package orchestrator

import (
	"strings"
	"testing"
)

func TestBuildShellCmd_NonSandbox(t *testing.T) {
	orchID := "orch-123"
	taskID := "20260406-120000-abc123"
	cmd := buildShellCmd(orchID, taskID, "/tmp/worktree", false, "")

	expected := "export CLAUDE_MUX_SESSION=orch-123/20260406-120000-abc123"
	if !strings.Contains(cmd, expected) {
		t.Errorf("expected command to contain %q, got:\n%s", expected, cmd)
	}

	if !strings.Contains(cmd, "claude --dangerously-skip-permissions") {
		t.Errorf("expected command to contain claude launch, got:\n%s", cmd)
	}
}

func TestBuildShellCmd_EnvFormat(t *testing.T) {
	cmd := buildShellCmd("abc", "def", "/tmp/wt", false, "")

	// The export should appear before the claude command
	exportIdx := strings.Index(cmd, "export CLAUDE_MUX_SESSION=abc/def")
	claudeIdx := strings.Index(cmd, "claude")
	if exportIdx == -1 || claudeIdx == -1 || exportIdx >= claudeIdx {
		t.Errorf("export should precede claude command, got:\n%s", cmd)
	}
}
