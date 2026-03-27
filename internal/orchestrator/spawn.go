package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Marcusk19/claude-mux/internal/kanban"
)

const subagentSystemPromptTmpl = `You are an implementation agent in a multi-agent swarm. You have been assigned a specific subtask.

## Your Role

- Read your task file at ` + "`.claude-mux/task.md`" + ` for your assignment
- Implement the task completely — write clean, working code
- Run tests if applicable (` + "`make build`" + `, ` + "`go test ./...`" + `, etc.)
- Commit your changes with a clear commit message when done
- Stay focused on your subtask — do not modify files outside your assignment unless necessary

## Guidelines

- Keep changes minimal and focused
- Do not refactor unrelated code
- If you encounter a blocker, commit what you have with a note in the commit message
- Your work will be validated by a separate agent after completion
`

// SpawnOpts configures a subagent spawn.
type SpawnOpts struct {
	Task         string   // required: task description
	Context      string   // optional: additional context
	Files        []string // optional: file paths to include in task file
	WorktreePath string   // optional: reuse an existing worktree instead of creating one
	BranchName   string   // optional: branch name for the existing worktree
	CardID       string   // optional: kanban card ID to move to in-progress
}

// Spawn creates a worktree, writes a task file, opens a tmux pane with claude, and saves state.
// Returns the task ID.
func Spawn(opts SpawnOpts) (string, error) {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return "", fmt.Errorf("resolving orchestrator ID: %w", err)
	}

	repoRoot, err := gitRepoRoot(".")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	taskID := generateTaskID()

	var absWorktree, branchName string

	if opts.WorktreePath != "" {
		// Reuse an existing worktree
		absWorktree, err = filepath.Abs(opts.WorktreePath)
		if err != nil {
			absWorktree = opts.WorktreePath
		}
		branchName = opts.BranchName
		if branchName == "" {
			// Detect branch from the worktree
			out, err := exec.Command("git", "-C", absWorktree, "rev-parse", "--abbrev-ref", "HEAD").Output()
			if err != nil {
				return "", fmt.Errorf("detecting branch in worktree: %w", err)
			}
			branchName = strings.TrimSpace(string(out))
		}
	} else {
		// Create a new worktree
		repoName := filepath.Base(repoRoot)
		branchName = "worktree/" + taskID
		worktreeDir := filepath.Join(filepath.Dir(repoRoot), repoName+"-wt-"+taskID)

		out, err := exec.Command("git", "worktree", "add", worktreeDir, "-b", branchName).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("creating worktree: %s: %s", err, strings.TrimSpace(string(out)))
		}

		absWorktree, err = filepath.Abs(worktreeDir)
		if err != nil {
			absWorktree = worktreeDir
		}
	}

	// Write task file
	if err := writeTaskFile(absWorktree, opts); err != nil {
		return "", fmt.Errorf("writing task file: %w", err)
	}

	// Write orchestrator metadata files for hook-driven completion
	board, boardErr := kanban.LoadBoard(repoRoot)
	if boardErr == nil {
		taskDir := filepath.Join(absWorktree, ".claude-mux")
		os.MkdirAll(taskDir, 0o755)
		os.WriteFile(filepath.Join(taskDir, "orchestrator-pane"), []byte(board.OrchestratorPaneID), 0o644)
		os.WriteFile(filepath.Join(taskDir, "board-path"), []byte(board.Path), 0o644)
	}

	// Open tmux pane with claude
	paneID, err := openClaudePane(absWorktree, orchID)
	if err != nil {
		return "", fmt.Errorf("opening tmux pane: %w", err)
	}

	// Save state
	state := SubagentState{
		TaskID:         taskID,
		OrchestratorID: orchID,
		Task:           opts.Task,
		WorktreePath:   absWorktree,
		BranchName:     branchName,
		RepoRoot:       repoRoot,
		PaneID:         paneID,
		Status:         "running",
		CreatedAt:      time.Now(),
	}
	if err := writeState(state); err != nil {
		return "", fmt.Errorf("writing state: %w", err)
	}

	// Move kanban card to in-progress
	if opts.CardID != "" && boardErr == nil {
		// Reload board in case it was modified
		board, err = kanban.LoadBoardFromPath(board.Path)
		if err == nil {
			card, _ := kanban.FindCardByID(board, opts.CardID)
			if card != nil {
				card.TaskID = taskID
				card.PaneID = paneID
				card.Branch = branchName
				kanban.MoveCard(board, opts.CardID, "in-progress")
				kanban.SaveBoard(board)
			}
		}
	}

	return taskID, nil
}

func writeTaskFile(worktreeDir string, opts SpawnOpts) error {
	taskDir := filepath.Join(worktreeDir, ".claude-mux")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# Task\n\n")
	b.WriteString(opts.Task)
	b.WriteString("\n")

	if opts.Context != "" {
		b.WriteString("\n## Context\n\n")
		b.WriteString(opts.Context)
		b.WriteString("\n")
	}

	for _, f := range opts.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		b.WriteString("\n## File: ")
		b.WriteString(f)
		b.WriteString("\n\n```\n")
		b.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	return os.WriteFile(filepath.Join(taskDir, "task.md"), []byte(b.String()), 0o644)
}

func paneExists(paneID string) bool {
	err := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{pane_id}").Run()
	return err == nil
}

func openClaudePane(worktreeDir string, orchID string) (string, error) {
	prompt := "Read .claude-mux/task.md and complete the task. Commit your changes when done."
	// Write prompt to a file to avoid shell quoting issues
	promptFile := filepath.Join(worktreeDir, ".claude-mux", "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(prompt), 0o644); err != nil {
		return "", fmt.Errorf("writing prompt file: %w", err)
	}

	// Write system prompt file
	systemPromptFile := filepath.Join(worktreeDir, ".claude-mux", "system-prompt.txt")
	if err := os.WriteFile(systemPromptFile, []byte(subagentSystemPromptTmpl), 0o644); err != nil {
		return "", fmt.Errorf("writing system prompt file: %w", err)
	}

	// Determine split direction and target based on existing live subagent panes
	states, _ := listStates(orchID)

	// Filter to live panes and sort by creation time
	var live []SubagentState
	for _, s := range states {
		if paneExists(s.PaneID) {
			live = append(live, s)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		return live[i].CreatedAt.Before(live[j].CreatedAt)
	})

	n := len(live)

	// Build tmux split-window args
	splitArgs := []string{"split-window"}
	switch {
	case n == 0:
		// First agent: vertical split, 50% below orchestrator
		splitArgs = append(splitArgs, "-v", "-l", "50%")
	case n%2 == 1:
		// Odd count (1, 3, 5...): horizontal split on last agent (pair up side by side)
		splitArgs = append(splitArgs, "-h", "-t", live[n-1].PaneID)
	default:
		// Even count > 0 (2, 4...): vertical split on last agent (new row below)
		splitArgs = append(splitArgs, "-v", "-t", live[n-1].PaneID)
	}

	// Launch claude interactively with the prompt as a positional argument.
	// Using --dangerously-skip-permissions for autonomous subagent work.
	shellCmd := `claude --dangerously-skip-permissions --append-system-prompt "$(cat .claude-mux/system-prompt.txt)" "$(cat .claude-mux/prompt.txt)"`

	splitArgs = append(splitArgs,
		"-c", worktreeDir,
		"-P", "-F", "#{pane_id}",
		shellCmd,
	)

	out, err := exec.Command("tmux", splitArgs...).Output()
	if err != nil {
		return "", err
	}
	newPaneID := strings.TrimSpace(string(out))

	// Equalize subagent pane heights when we have 3+ agents (n >= 2 before this spawn)
	if n >= 2 {
		allPaneIDs := make([]string, len(live)+1)
		for i, s := range live {
			allPaneIDs[i] = s.PaneID
		}
		allPaneIDs[len(live)] = newPaneID
		equalizeSubagentPanes(allPaneIDs)
	}

	return newPaneID, nil
}

func equalizeSubagentPanes(paneIDs []string) {
	// Get pane positions from tmux
	out, err := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_top} #{window_height}").Output()
	if err != nil {
		return
	}

	type paneInfo struct {
		id  string
		top int
	}

	// Parse pane info, filtering to our subagent panes
	paneSet := make(map[string]bool, len(paneIDs))
	for _, id := range paneIDs {
		paneSet[id] = true
	}

	var windowHeight int
	var subagentPanes []paneInfo
	minTop := -1

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		id := fields[0]
		top, _ := strconv.Atoi(fields[1])
		wh, _ := strconv.Atoi(fields[2])
		if wh > windowHeight {
			windowHeight = wh
		}
		if paneSet[id] {
			subagentPanes = append(subagentPanes, paneInfo{id: id, top: top})
			if minTop == -1 || top < minTop {
				minTop = top
			}
		}
	}

	if len(subagentPanes) == 0 || windowHeight == 0 {
		return
	}

	// Group by pane_top to identify rows
	rows := make(map[int][]paneInfo)
	for _, p := range subagentPanes {
		rows[p.top] = append(rows[p.top], p)
	}

	numRows := len(rows)
	if numRows == 0 {
		return
	}

	// Orchestrator occupies everything above the first subagent row
	orchHeight := minTop
	availableHeight := windowHeight - orchHeight
	heightPerRow := availableHeight / numRows

	if heightPerRow < 1 {
		return
	}

	// Resize one representative pane per row
	for _, panes := range rows {
		if len(panes) > 0 {
			exec.Command("tmux", "resize-pane", "-t", panes[0].id, "-y", strconv.Itoa(heightPerRow)).Run()
		}
	}
}

// RepoRoot returns the git repository root for the current directory.
func RepoRoot() (string, error) {
	return gitRepoRoot(".")
}

func gitRepoRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
