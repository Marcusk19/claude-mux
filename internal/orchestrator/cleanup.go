package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Marcusk19/claude-mux/internal/worktree"
)

// CleanupOpts configures cleanup behavior.
type CleanupOpts struct {
	TaskID string // optional: clean up a specific subagent
	Force  bool   // remove running subagents too
}

// Cleanup removes worktrees, branches, and state files for completed subagents.
func Cleanup(opts CleanupOpts) error {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return err
	}

	states, err := listStates(orchID)
	if err != nil {
		return err
	}

	for _, s := range states {
		if opts.TaskID != "" && s.TaskID != opts.TaskID {
			continue
		}
		if !opts.Force && s.Status == "running" {
			continue
		}

		// Remove worktree
		if err := worktree.Remove(s.RepoRoot, s.WorktreePath, true); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing worktree %s: %v\n", s.WorktreePath, err)
		}

		// Delete branch
		exec.Command("git", "-C", s.RepoRoot, "branch", "-D", s.BranchName).Run()

		// Remove state file
		deleteState(orchID, s.TaskID)
	}

	return nil
}

// FormatCleanup returns a summary of what was cleaned up.
func FormatCleanup(states []SubagentState, opts CleanupOpts) string {
	var cleaned []string
	for _, s := range states {
		if opts.TaskID != "" && s.TaskID != opts.TaskID {
			continue
		}
		if !opts.Force && s.Status == "running" {
			continue
		}
		cleaned = append(cleaned, s.TaskID)
	}
	if len(cleaned) == 0 {
		return "Nothing to clean up."
	}
	return fmt.Sprintf("Cleaned up %d subagent(s): %s", len(cleaned), strings.Join(cleaned, ", "))
}
