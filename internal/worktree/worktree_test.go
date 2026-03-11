package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a bare-minimum git repo with one commit at dir.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// TestDiscoverWorktrees_SeparateCloneIsNotWorktree verifies that an
// independent clone of the same repo does not appear as a worktree entry
// alongside actual worktrees. Each clone has its own worktree list, and
// they must not cross-contaminate.
func TestDiscoverWorktrees_SeparateCloneIsNotWorktree(t *testing.T) {
	rawTmp := t.TempDir()
	// Resolve symlinks so paths match what git rev-parse returns (macOS /var -> /private/var).
	tmp, err := filepath.EvalSymlinks(rawTmp)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Create the "origin" repo and two independent clones.
	origin := filepath.Join(tmp, "origin")
	cloneA := filepath.Join(tmp, "clone-a")
	cloneB := filepath.Join(tmp, "clone-b")

	initRepo(t, origin)

	for _, dest := range []string{cloneA, cloneB} {
		if out, err := exec.Command("git", "clone", origin, dest).CombinedOutput(); err != nil {
			t.Fatalf("git clone: %v\n%s", err, out)
		}
	}

	// Add a real worktree to clone-a.
	wtPath := filepath.Join(tmp, "clone-a-wt")
	if out, err := exec.Command("git", "-C", cloneA, "worktree", "add", wtPath, "-b", "wt-branch").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	// Discover worktrees from pane paths that include both clones and the worktree.
	wts, err := DiscoverWorktrees([]string{cloneA, cloneB, wtPath})
	if err != nil {
		t.Fatalf("DiscoverWorktrees: %v", err)
	}

	// Collect which paths appeared and which repo root they belong to.
	type entry struct {
		path     string
		repoRoot string
		isMain   bool
	}
	var entries []entry
	for _, wt := range wts {
		entries = append(entries, entry{wt.Path, wt.RepoRoot, wt.IsMain})
	}

	// clone-a should have exactly 2 entries: the main worktree and the added worktree.
	var cloneAEntries []entry
	for _, e := range entries {
		if e.repoRoot == cloneA {
			cloneAEntries = append(cloneAEntries, e)
		}
	}
	if len(cloneAEntries) != 2 {
		t.Fatalf("expected 2 worktrees for clone-a, got %d: %+v", len(cloneAEntries), cloneAEntries)
	}

	// One must be the main worktree, one must be the added worktree.
	var foundMain, foundWT bool
	for _, e := range cloneAEntries {
		if e.path == cloneA && e.isMain {
			foundMain = true
		}
		if e.path == wtPath && !e.isMain {
			foundWT = true
		}
	}
	if !foundMain {
		t.Errorf("clone-a main worktree not found in results: %+v", cloneAEntries)
	}
	if !foundWT {
		t.Errorf("clone-a added worktree (%s) not found in results: %+v", wtPath, cloneAEntries)
	}

	// clone-b should have exactly 1 entry: its own main worktree.
	// It must NOT contain clone-a's worktree or clone-a itself.
	var cloneBEntries []entry
	for _, e := range entries {
		if e.repoRoot == cloneB {
			cloneBEntries = append(cloneBEntries, e)
		}
	}
	if len(cloneBEntries) != 1 {
		t.Fatalf("expected 1 worktree for clone-b, got %d: %+v", len(cloneBEntries), cloneBEntries)
	}
	if cloneBEntries[0].path != cloneB || !cloneBEntries[0].isMain {
		t.Errorf("clone-b entry should be its own main worktree, got %+v", cloneBEntries[0])
	}

	// The added worktree must NOT appear under clone-b's repo root.
	for _, e := range entries {
		if e.path == wtPath && e.repoRoot == cloneB {
			t.Errorf("worktree %s incorrectly associated with clone-b", wtPath)
		}
	}
}
