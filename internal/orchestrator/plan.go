package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/Marcusk19/claude-mux/internal/kanban"
)

// PlanOpts configures a plan session.
type PlanOpts struct {
	Task      string // optional: initial task idea
	Context   string // optional: additional context
	Files     []string
	AutoMerge bool
	MaxAgents int
	Sandbox   bool   // run subagents in sandboxed containers
	RepoDir   string // optional: git repo directory (defaults to cwd)
}

// Plan launches an interactive Claude session that combines planning and
// orchestration. The user iterates on a PRD, then the same session spawns
// subagents directly — no separate coordinator session.
// This function replaces the current process (exec).
func Plan(opts PlanOpts) error {
	repoDir := opts.RepoDir
	if repoDir == "" {
		repoDir = "."
	}
	repoRoot, err := gitRepoRoot(repoDir)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	planID := generateTaskID()

	if opts.MaxAgents <= 0 {
		opts.MaxAgents = 3
	}

	// Create plan directory
	planDir := filepath.Join(repoRoot, ".claude-mux", "plan-"+planID)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return fmt.Errorf("creating plan directory: %w", err)
	}

	// Capture current pane ID for hook notifications
	paneID := currentPaneID()

	// Set up orchestrator infrastructure so spawn/hooks work immediately
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return fmt.Errorf("resolving orchestrator ID: %w", err)
	}

	// Create swarm directory with kanban board
	swarmDir := filepath.Join(repoRoot, ".claude-mux", "swarm-"+planID)
	if err := os.MkdirAll(swarmDir, 0o755); err != nil {
		return fmt.Errorf("creating swarm directory: %w", err)
	}

	board := kanban.NewBoard(planID, paneID)
	boardPath := filepath.Join(swarmDir, "kanban.json")
	board.Path = boardPath
	if err := kanban.SaveBoard(board); err != nil {
		return fmt.Errorf("saving board: %w", err)
	}

	os.WriteFile(filepath.Join(swarmDir, "board-path"), []byte(boardPath), 0o644)

	orchIDFile := filepath.Join(repoRoot, ".claude-mux", "orchestrator-id")
	os.WriteFile(orchIDFile, []byte(orchID+"\n"), 0o644)

	// Build file context
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

	// Render system prompt
	sysTmpl, err := template.New("system").Parse(unifiedPlannerSystemPromptTmpl)
	if err != nil {
		return fmt.Errorf("parsing system prompt template: %w", err)
	}

	sysData := struct {
		PlanDir   string
		AutoMerge bool
		MaxAgents int
		Task      string
		Context   string
		Sandbox   bool
	}{
		PlanDir:   filepath.Join(".claude-mux", "plan-"+planID),
		AutoMerge: opts.AutoMerge,
		MaxAgents: opts.MaxAgents,
		Task:      opts.Task,
		Context:   contextBuf.String(),
		Sandbox:   opts.Sandbox,
	}

	systemPromptFile := filepath.Join(planDir, "system-prompt.txt")
	sf, err := os.Create(systemPromptFile)
	if err != nil {
		return fmt.Errorf("creating system prompt file: %w", err)
	}
	if err := sysTmpl.Execute(sf, sysData); err != nil {
		sf.Close()
		return fmt.Errorf("rendering system prompt: %w", err)
	}
	sf.Close()

	// Build claude command args
	systemPrompt, err := os.ReadFile(systemPromptFile)
	if err != nil {
		return fmt.Errorf("reading system prompt: %w", err)
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Plan session %s — explore the codebase, iterate on a plan, then say \"execute\" to start spawning agents.\n", planID)

	// Replace current process with claude (no initial message — user drives the conversation)
	args := []string{
		"claude",
		"--append-system-prompt", string(systemPrompt),
	}

	return syscall.Exec(claudeBin, args, os.Environ())
}

// currentPaneID returns the tmux pane ID of the current terminal.
func currentPaneID() string {
	if id := os.Getenv("TMUX_PANE"); id != "" {
		return id
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{pane_id}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

const unifiedPlannerSystemPromptTmpl = `You are a project planner and swarm coordinator. You have two phases:

## Phase 1: Planning (current)

You are collaborative and conversational. Your job is to:

1. **Understand the goal**: Ask clarifying questions about what the user wants to build or change. Don't assume — dig into requirements, edge cases, and constraints.

   When asking clarifying questions, use the AskUserQuestion tool rather than inline text. This provides a structured UI for the user to select options and makes the conversation more interactive. Use it for:
   - Design decisions with discrete options
   - Confirming requirements before proceeding
   - Getting user preferences on implementation approaches

2. **Explore the codebase**: Read relevant files, understand the architecture, and identify what needs to change. Reference specific files and code.

3. **Draft the PRD**: Once you have enough understanding, write a structured PRD that covers:
   - **Goal**: What we're building and why
   - **Scope**: What's in and out of scope
   - **Architecture**: How it fits into the existing codebase
   - **Subtasks**: A numbered list of 2-6 independent implementation tasks, each with:
     - Clear description of what to implement
     - Which files to create or modify
     - Acceptance criteria
   - **Testing**: How to verify the work
   - **Risks**: Potential issues or conflicts between subtasks

4. **Iterate**: The user may push back, ask for changes, or want to explore alternatives. Revise the PRD until they're happy.

5. **Execute**: When the user approves (says "execute", "ship it", "go", "lgtm", or similar), transition to Phase 2.

## Phase 2: Execution

When the user approves the plan:

1. **Save the PRD** to ` + "`" + `{{.PlanDir}}/prd.md` + "`" + `

2. **Spawn implementers** for each subtask using:
   ` + "`" + `claude-mux spawn --task "implement: <detailed subtask description>" --card-id <card-id>{{if .Sandbox}} --sandbox{{end}}` + "`" + `

   Spawn at most {{.MaxAgents}} agents at a time. If you have more subtasks, wait for some to complete before spawning more.

3. **Wait for notifications**: Agents will notify you when they complete. You will receive a message like:
   ` + "`" + `Agent <task-id> completed on branch <branch>` + "`" + `
   Wait for these messages instead of polling. You can run ` + "`" + `claude-mux status` + "`" + ` as a fallback if needed.

4. **Validate completed work**: When an implementer completes, spawn a validator in the same worktree:
   ` + "`" + `claude-mux spawn --task "validate: review the changes, run tests, and verify correctness" --worktree <worktree_path> --branch <branch_name>` + "`" + `

5. **Collect results** when all implementers and validators are done:
{{- if .AutoMerge}}
   ` + "`" + `claude-mux collect --merge` + "`" + `
{{- else}}
   ` + "`" + `claude-mux collect` + "`" + `
{{- end}}

6. **Report a summary** of what was accomplished, including:
   - Which subtasks succeeded or failed
   - Key changes made in each branch
   - Any issues found during validation
   - Whether branches were merged (if auto-merge was enabled)

## Guidelines

- **During Phase 1**: Do not edit files, create files (except the PRD), or run non-read commands. Your output is the PRD.
- **During Phase 2**: Do NOT modify code directly. Your only job is to coordinate subagents.
- Keep subtasks **independent** to avoid merge conflicts between agents. Each subtask should touch different files where possible.
- Be opinionated — suggest good approaches rather than listing options endlessly.
- If the task is small enough for one agent, say so. Not everything needs a swarm.
- Reference actual code paths and function names from the codebase.
- The PRD should be detailed enough that an AI agent can implement each subtask without further clarification.
- If a subtask fails, you may retry it once with a refined task description.
- If validation fails, spawn a new implementer to fix the issues in the same worktree.
{{- if .Task}}

## User's Task

The user wants to work on: {{.Task}}
{{- end}}
{{- if .Context}}

## Provided Context

{{.Context}}
{{- end}}
`
