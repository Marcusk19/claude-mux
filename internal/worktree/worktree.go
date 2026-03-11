package worktree

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree represents a git worktree entry.
type Worktree struct {
	Path       string // absolute worktree path
	Branch     string // branch name or "(detached)"
	Head       string // short commit SHA
	RepoRoot   string // main repo this belongs to
	IsMain     bool   // true for the primary worktree
	HasSession bool   // set externally: Claude session active here
	PaneID     string // set externally: tmux pane ID if session active
}

// DiscoverWorktrees finds all git worktrees for repos visible in the given pane paths.
func DiscoverWorktrees(panePaths []string) ([]Worktree, error) {
	roots := make(map[string]bool)
	for _, p := range panePaths {
		root, err := repoRoot(p)
		if err != nil {
			continue
		}
		roots[root] = true
	}

	var all []Worktree
	for root := range roots {
		out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
		if err != nil {
			continue
		}
		wts := parseWorktreeList(string(out), root)
		// Skip repos that only have the main worktree (no actual worktrees created)
		if len(wts) <= 1 {
			continue
		}
		all = append(all, wts...)
	}
	return all, nil
}

// Remove removes a git worktree.
func Remove(repoRoot, path string, force bool) error {
	args := []string{"-C", repoRoot, "worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func repoRoot(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseWorktreeList(output string, repoRoot string) []Worktree {
	var worktrees []Worktree
	var current *Worktree

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			current = &Worktree{
				Path:     path,
				RepoRoot: repoRoot,
				IsMain:   path == repoRoot,
			}
		} else if strings.HasPrefix(line, "HEAD ") && current != nil {
			sha := strings.TrimPrefix(line, "HEAD ")
			if len(sha) > 7 {
				sha = sha[:7]
			}
			current.Head = sha
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		} else if line == "detached" && current != nil {
			current.Branch = "(detached)"
		}
	}
	// Handle last entry if no trailing blank line
	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees
}

// RepoName returns the base directory name of the repo root.
func RepoName(repoRoot string) string {
	return filepath.Base(repoRoot)
}
