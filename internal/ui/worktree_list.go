package ui

import (
	"fmt"

	"github.com/Marcusk19/claude-mux/internal/worktree"
)

// worktreeItem implements the list.DefaultItem interface for the worktree list.
type worktreeItem struct {
	wt worktree.Worktree
}

func (i worktreeItem) Title() string {
	repoName := worktree.RepoName(i.wt.RepoRoot)
	title := fmt.Sprintf("%s (%s)", i.wt.Branch, repoName)

	if i.wt.IsMain {
		title += " [main]"
	}
	if i.wt.HasSession {
		title = "* " + title
	}
	return title
}

func (i worktreeItem) Description() string {
	path := shortenPath(i.wt.Path)
	desc := fmt.Sprintf("%s  %s", path, i.wt.Head)
	return desc
}

func (i worktreeItem) FilterValue() string {
	return i.wt.Path + " " + i.wt.Branch
}
