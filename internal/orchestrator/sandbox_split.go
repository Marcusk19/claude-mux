package orchestrator

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Marcusk19/claude-mux/internal/openshell"
)

// SandboxSplitOpts configures a sandboxed split pane.
type SandboxSplitOpts struct {
	SplitFlag string // "-h" (vertical/side-by-side) or "-v" (horizontal/stacked)
	PanePath  string // working directory of the current pane
	Provider  string // openshell provider name (empty = use openshell default)
}

// SandboxSplit creates an openshell sandbox and opens a tmux split with
// an interactive Claude session inside it. The current directory is uploaded
// into the sandbox — no worktree is created.
func SandboxSplit(opts SandboxSplitOpts) error {
	if !openshell.Available() {
		return tmuxMessage("sandbox requires openshell (not found in PATH)")
	}

	absPath, err := filepath.Abs(opts.PanePath)
	if err != nil {
		absPath = opts.PanePath
	}

	sandboxName := "claude-mux-" + generateTaskID()
	shellCmd := openshell.BuildInteractiveCommand(sandboxName, absPath, opts.Provider)

	splitArgs := []string{
		"split-window",
		opts.SplitFlag,
		"-c", absPath,
		shellCmd,
	}

	if out, err := exec.Command("tmux", splitArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux split-window: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func tmuxMessage(msg string) error {
	return exec.Command("tmux", "display-message", "claude-mux: "+msg).Run()
}
