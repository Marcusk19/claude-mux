package orchestrator

import (
	"fmt"
	"os/exec"
	"strings"
)

// CollectOpts configures result collection.
type CollectOpts struct {
	TaskID  string // optional: collect a specific subagent
	Merge   bool   // merge completed branches into current branch
	Cleanup bool   // remove worktrees after collecting
}

// CollectResult holds the collected output for a subagent.
type CollectResult struct {
	TaskID     string
	BranchName string
	DiffStat   string
	Log        string
	Merged     bool
	MergeError string
}

// Collect gathers results from completed subagents.
func Collect(opts CollectOpts) ([]CollectResult, error) {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return nil, err
	}

	states, err := listStates(orchID)
	if err != nil {
		return nil, err
	}

	// First, refresh statuses (detect completion for panes that exited)
	refreshStatuses(orchID, states)

	// Re-read after refresh
	states, err = listStates(orchID)
	if err != nil {
		return nil, err
	}

	var results []CollectResult
	for _, s := range states {
		if opts.TaskID != "" && s.TaskID != opts.TaskID {
			continue
		}
		if s.Status != "completed" {
			continue
		}

		r := CollectResult{
			TaskID:     s.TaskID,
			BranchName: s.BranchName,
		}

		// Get diff stat
		out, err := exec.Command(
			"git", "-C", s.RepoRoot,
			"diff", "--stat", "HEAD..."+s.BranchName,
		).Output()
		if err == nil {
			r.DiffStat = strings.TrimSpace(string(out))
		}

		// Get commit log
		out, err = exec.Command(
			"git", "-C", s.RepoRoot,
			"log", "--oneline", "HEAD.."+s.BranchName,
		).Output()
		if err == nil {
			r.Log = strings.TrimSpace(string(out))
		}

		// Merge if requested
		if opts.Merge {
			mergeOut, mergeErr := exec.Command(
				"git", "-C", s.RepoRoot,
				"merge", s.BranchName, "--no-edit",
			).CombinedOutput()
			if mergeErr != nil {
				r.MergeError = strings.TrimSpace(string(mergeOut))
			} else {
				r.Merged = true
			}
		}

		results = append(results, r)

		// Cleanup if requested
		if opts.Cleanup {
			Cleanup(CleanupOpts{TaskID: s.TaskID})
		}
	}

	return results, nil
}

// FormatCollect produces human-readable output for collected results.
func FormatCollect(results []CollectResult) string {
	if len(results) == 0 {
		return "No completed subagents to collect."
	}

	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "=== %s (%s) ===\n", r.TaskID, r.BranchName)

		if r.Log != "" {
			fmt.Fprintf(&b, "\nCommits:\n%s\n", r.Log)
		}
		if r.DiffStat != "" {
			fmt.Fprintf(&b, "\nChanges:\n%s\n", r.DiffStat)
		}
		if r.Merged {
			b.WriteString("\nMerged: yes\n")
		}
		if r.MergeError != "" {
			fmt.Fprintf(&b, "\nMerge error: %s\n", r.MergeError)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// refreshStatuses updates status for subagents whose panes may have exited.
func refreshStatuses(orchID string, states []SubagentState) {
	// Just call Status() which handles the refresh logic
	Status()
}
