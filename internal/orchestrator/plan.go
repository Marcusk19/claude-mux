package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
)

// PlanOpts configures a plan session.
type PlanOpts struct {
	Task      string // optional: initial task idea
	Context   string // optional: additional context
	Files     []string
	AutoMerge bool
	MaxAgents int
}

// Plan launches an interactive Claude session for collaborative PRD writing.
// When the user is satisfied with the PRD, Claude saves it and kicks off a swarm.
// This function replaces the current process (exec).
func Plan(opts PlanOpts) error {
	repoRoot, err := gitRepoRoot(".")
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
	sysTmpl, err := template.New("system").Parse(plannerSystemPromptTmpl)
	if err != nil {
		return fmt.Errorf("parsing system prompt template: %w", err)
	}

	sysData := struct {
		PlanDir   string
		PlanID    string
		AutoMerge bool
		MaxAgents int
		Task      string
		Context   string
	}{
		PlanDir:   filepath.Join(".claude-mux", "plan-"+planID),
		PlanID:    planID,
		AutoMerge: opts.AutoMerge,
		MaxAgents: opts.MaxAgents,
		Task:      opts.Task,
		Context:   contextBuf.String(),
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

	fmt.Fprintf(os.Stderr, "Plan session %s — explore the codebase, iterate on a plan, then say \"execute\" to hand off to the swarm.\n", planID)

	// Replace current process with claude (no initial message — user drives the conversation)
	args := []string{
		"claude",
		"--append-system-prompt", string(systemPrompt),
	}

	return syscall.Exec(claudeBin, args, os.Environ())
}

const plannerSystemPromptTmpl = `You are a project planner. You operate in planning-only mode: you read files, explore the codebase, ask questions, and iterate on a PRD — but you do NOT edit code or implement anything yourself. Implementation is handed off to a swarm of AI coding agents.

## Your Role

You are collaborative and conversational. Your job is to:

1. **Understand the goal**: Ask clarifying questions about what the user wants to build or change. Don't assume — dig into requirements, edge cases, and constraints.

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

5. **Hand off to the swarm**: When the user approves (says "execute", "ship it", "go", "lgtm", or similar), do the following:
   - Save the final PRD to ` + "`" + `{{.PlanDir}}/prd.md` + "`" + `
   - Then hand off to the orchestrator by running:
     ` + "`" + `claude-mux swarm --task "<one-line summary>" --file {{.PlanDir}}/prd.md{{if .AutoMerge}} --auto-merge{{end}} --max-agents {{.MaxAgents}}` + "`" + `
   - Do NOT attempt to implement the plan yourself. The swarm handles all implementation.

## Guidelines

- **Planning only** — do not edit files, create files, or run non-read commands. Your output is the PRD.
- Keep subtasks **independent** to avoid merge conflicts between agents. Each subtask should touch different files where possible.
- Be opinionated — suggest good approaches rather than listing options endlessly.
- If the task is small enough for one agent, say so. Not everything needs a swarm.
- Reference actual code paths and function names from the codebase.
- The PRD should be detailed enough that an AI agent can implement each subtask without further clarification.
{{- if .Task}}

## User's Task

The user wants to work on: {{.Task}}
{{- end}}
{{- if .Context}}

## Provided Context

{{.Context}}
{{- end}}
`
