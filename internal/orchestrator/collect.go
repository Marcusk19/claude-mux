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
	// Use Status() to get live state (including hook enrichment and pane exit detection)
	statusResults, err := Status()
	if err != nil {
		return nil, err
	}

	var results []CollectResult
	for _, sr := range statusResults {
		s := sr.SubagentState
		if opts.TaskID != "" && s.TaskID != opts.TaskID {
			continue
		}
		// Treat as collectable if:
		// 1. Persisted status is "completed" (pane exited with commits), or
		// 2. Hook reports "done", or
		// 3. Branch has commits beyond HEAD (work was committed even if pane/hook are stale)
		isCompleted := s.Status == "completed" || sr.LiveStatus == "done"
		if !isCompleted {
			// Check if branch has commits as a fallback
			if detectCompletion(s) != "completed" {
				continue
			}
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

		// Kill the pane if it's still open
		exec.Command("tmux", "kill-pane", "-t", s.PaneID).Run()

		// Mark as completed in state
		if s.Status == "running" {
			s.Status = "completed"
			writeState(s)
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

