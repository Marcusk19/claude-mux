package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"text/template"

	"github.com/Marcusk19/claude-mux/internal/kanban"
)

// SwarmOpts configures a swarm launch.
type SwarmOpts struct {
	Task      string   // required: high-level task description
	Context   string   // optional: additional context
	Files     []string // optional: file paths to include
	AutoMerge bool     // whether to auto-merge completed branches
	MaxAgents int      // max concurrent subagents (default 3)
}

// Swarm launches a coordinator Claude session that breaks a task into subtasks
// and manages subagents. This function replaces the current process (exec).
func Swarm(opts SwarmOpts) error {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return fmt.Errorf("resolving orchestrator ID: %w", err)
	}

	repoRoot, err := gitRepoRoot(".")
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	swarmID := generateTaskID()

	if opts.MaxAgents <= 0 {
		opts.MaxAgents = 3
	}

	// Capture current pane ID for hook notifications
	paneID := currentPaneID()

	// Create swarm directory
	swarmDir := filepath.Join(repoRoot, ".claude-mux", "swarm-"+swarmID)
	if err := os.MkdirAll(swarmDir, 0o755); err != nil {
		return fmt.Errorf("creating swarm directory: %w", err)
	}

	// Build context with optional files
	var contextBuf strings.Builder
	if opts.Context != "" {
		contextBuf.WriteString(opts.Context)
		contextBuf.WriteString("\n")
	}
	for _, f := range opts.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		contextBuf.WriteString("\n## File: ")
		contextBuf.WriteString(f)
		contextBuf.WriteString("\n\n```\n")
		contextBuf.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			contextBuf.WriteString("\n")
		}
		contextBuf.WriteString("```\n")
	}

	// Select templates based on whether files (PRD) were provided
	var sysTmplStr, msgTmplStr string
	if len(opts.Files) > 0 {
		sysTmplStr = coordinatorPlanDrivenSystemPromptTmpl
		msgTmplStr = coordinatorPlanDrivenInitialMessageTmpl
	} else {
		sysTmplStr = coordinatorSystemPromptTmpl
		msgTmplStr = coordinatorInitialMessageTmpl
	}

	// Render system prompt (coordinator instructions)
	sysTmpl, err := template.New("system").Parse(sysTmplStr)
	if err != nil {
		return fmt.Errorf("parsing system prompt template: %w", err)
	}

	sysData := struct {
		AutoMerge bool
		MaxAgents int
	}{
		AutoMerge: opts.AutoMerge,
		MaxAgents: opts.MaxAgents,
	}

	systemPromptFile := filepath.Join(swarmDir, "system-prompt.txt")
	sf, err := os.Create(systemPromptFile)
	if err != nil {
		return fmt.Errorf("creating system prompt file: %w", err)
	}
	if err := sysTmpl.Execute(sf, sysData); err != nil {
		sf.Close()
		return fmt.Errorf("rendering system prompt: %w", err)
	}
	sf.Close()

	// Render initial message (task + context)
	msgTmpl, err := template.New("message").Parse(msgTmplStr)
	if err != nil {
		return fmt.Errorf("parsing initial message template: %w", err)
	}

	msgData := struct {
		Task    string
		Context string
	}{
		Task:    opts.Task,
		Context: contextBuf.String(),
	}

	initialMessageFile := filepath.Join(swarmDir, "initial-message.txt")
	mf, err := os.Create(initialMessageFile)
	if err != nil {
		return fmt.Errorf("creating initial message file: %w", err)
	}
	if err := msgTmpl.Execute(mf, msgData); err != nil {
		mf.Close()
		return fmt.Errorf("rendering initial message: %w", err)
	}
	mf.Close()

	// Create kanban board
	board := kanban.NewBoard(swarmID, paneID)

	// If PRD files were provided, parse subtasks from content
	if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			cards := parseSubtasks(string(data))
			board.Columns["backlog"] = append(board.Columns["backlog"], cards...)
		}
	}

	// Save board
	boardPath := filepath.Join(swarmDir, "kanban.json")
	board.Path = boardPath
	if err := kanban.SaveBoard(board); err != nil {
		return fmt.Errorf("saving board: %w", err)
	}

	// Write board path reference
	os.WriteFile(filepath.Join(swarmDir, "board-path"), []byte(boardPath), 0o644)

	// Write orchestrator-id so spawn resolves to the same orchestrator
	orchIDFile := filepath.Join(repoRoot, ".claude-mux", "orchestrator-id")
	os.WriteFile(orchIDFile, []byte(orchID+"\n"), 0o644)

	// Read rendered files for claude args
	systemPrompt, err := os.ReadFile(systemPromptFile)
	if err != nil {
		return fmt.Errorf("reading system prompt: %w", err)
	}

	initialMessage, err := os.ReadFile(initialMessageFile)
	if err != nil {
		return fmt.Errorf("reading initial message: %w", err)
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Swarm %s (orchestrator: %s)\n", swarmID, orchID)

	// Replace current process with claude
	args := []string{
		"claude",
		"--append-system-prompt", string(systemPrompt),
		string(initialMessage),
	}

	return syscall.Exec(claudeBin, args, os.Environ())
}

const coordinatorSystemPromptTmpl = `You are a swarm coordinator. Your job is to break down a task into independent subtasks, spawn subagents to implement them, validate results, and collect everything when done.

## Workflow

1. **Propose a plan**: Analyze the task and present a numbered list of 2-6 subtasks. Explain what each subtask will do and why they are independent. Wait for the user to approve or adjust the plan.

2. **Execute on approval**: Once the user approves, spawn implementers and monitor progress as described below.

## Spawning and monitoring

- **Spawn implementers** for each subtask using:
  ` + "`" + `claude-mux spawn --task "implement: <detailed subtask description>" --card-id <card-id>` + "`" + `

  Spawn at most {{.MaxAgents}} agents at a time. If you have more subtasks than that, wait for some to complete before spawning more.

- **Wait for notifications**: Agents will notify you when they complete. You will receive a message like:
  ` + "`" + `Agent <task-id> completed on branch <branch>` + "`" + `
  Wait for these messages instead of polling. You can run ` + "`" + `claude-mux status` + "`" + ` as a fallback if needed.

- **Validate completed work**: When an implementer completes, spawn a validator in the same worktree:
  ` + "`" + `claude-mux spawn --task "validate: review the changes, run tests, and verify correctness" --worktree <worktree_path> --branch <branch_name>` + "`" + `

- **Collect results** when all implementers and validators are done:
{{- if .AutoMerge}}
  ` + "`" + `claude-mux collect --merge` + "`" + `
{{- else}}
  ` + "`" + `claude-mux collect` + "`" + `
{{- end}}

- **Report a summary** of what was accomplished, including:
  - Which subtasks succeeded or failed
  - Key changes made in each branch
  - Any issues found during validation
  - Whether branches were merged (if auto-merge was enabled)

## Rules

- Do NOT modify code directly. Your only job is to coordinate subagents.
- Keep subtasks focused and independent to minimize merge conflicts.
- If a subtask fails, you may retry it once with a refined task description.
- If validation fails, spawn a new implementer to fix the issues in the same worktree.
`

const coordinatorInitialMessageTmpl = `## Task

{{.Task}}
{{- if .Context}}

## Context

{{.Context}}
{{- end}}

Please analyze this task and propose a plan with subtasks before starting execution.
`

const coordinatorPlanDrivenSystemPromptTmpl = `You are a swarm coordinator. A PRD (Product Requirements Document) with approved subtasks has been provided. Your job is to read the PRD, extract the subtasks, spawn subagents to implement them, validate results, and collect everything when done.

## Workflow

1. **Read the PRD**: Parse the provided document and extract the approved subtasks.

2. **Execute immediately**: Spawn implementers for each subtask right away — the plan has already been approved.

## Spawning and monitoring

- **Spawn implementers** for each subtask using:
  ` + "`" + `claude-mux spawn --task "implement: <detailed subtask description>" --card-id <card-id>` + "`" + `

  Spawn at most {{.MaxAgents}} agents at a time. If you have more subtasks than that, wait for some to complete before spawning more.

- **Wait for notifications**: Agents will notify you when they complete. You will receive a message like:
  ` + "`" + `Agent <task-id> completed on branch <branch>` + "`" + `
  Wait for these messages instead of polling. You can run ` + "`" + `claude-mux status` + "`" + ` as a fallback if needed.

- **Validate completed work**: When an implementer completes, spawn a validator in the same worktree:
  ` + "`" + `claude-mux spawn --task "validate: review the changes, run tests, and verify correctness" --worktree <worktree_path> --branch <branch_name>` + "`" + `

- **Collect results** when all implementers and validators are done:
{{- if .AutoMerge}}
  ` + "`" + `claude-mux collect --merge` + "`" + `
{{- else}}
  ` + "`" + `claude-mux collect` + "`" + `
{{- end}}

- **Report a summary** of what was accomplished, including:
  - Which subtasks succeeded or failed
  - Key changes made in each branch
  - Any issues found during validation
  - Whether branches were merged (if auto-merge was enabled)

## Rules

- Do NOT modify code directly. Your only job is to coordinate subagents.
- Keep subtasks focused and independent to minimize merge conflicts.
- If a subtask fails, you may retry it once with a refined task description.
- If validation fails, spawn a new implementer to fix the issues in the same worktree.
`

const coordinatorPlanDrivenInitialMessageTmpl = `## Task

{{.Task}}
{{- if .Context}}

## Context

{{.Context}}
{{- end}}

A PRD has been approved. Read it and execute — spawn agents for each subtask immediately.
`

// parseSubtasks extracts numbered subtasks from a PRD document.
// It looks for a "### Subtasks" or "## Subtasks" header, then collects numbered items.
func parseSubtasks(content string) []kanban.Card {
	var cards []kanban.Card

	subtasksHeader := regexp.MustCompile(`(?i)^#{2,3}\s+Subtasks\s*$`)
	numberedItem := regexp.MustCompile(`^(\d+)\.\s+(.+)`)
	sectionHeader := regexp.MustCompile(`^#{1,6}\s+`)

	scanner := bufio.NewScanner(strings.NewReader(content))
	inSubtasks := false
	var currentID, currentTitle string
	var descLines []string

	flush := func() {
		if currentID != "" {
			cards = append(cards, kanban.Card{
				ID:          currentID,
				Title:       currentTitle,
				Description: strings.TrimSpace(strings.Join(descLines, "\n")),
			})
		}
		currentID = ""
		currentTitle = ""
		descLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		if !inSubtasks {
			if subtasksHeader.MatchString(line) {
				inSubtasks = true
			}
			continue
		}

		// Stop at next section header (that isn't "Subtasks")
		if sectionHeader.MatchString(line) && !subtasksHeader.MatchString(line) {
			flush()
			break
		}

		if m := numberedItem.FindStringSubmatch(line); m != nil {
			flush()
			currentID = m[1]
			currentTitle = m[2]
		} else if currentID != "" {
			descLines = append(descLines, line)
		}
	}
	flush()

	return cards
}
