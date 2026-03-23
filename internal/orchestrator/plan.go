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
	}{
		PlanDir:   filepath.Join(".claude-mux", "plan-"+planID),
		PlanID:    planID,
		AutoMerge: opts.AutoMerge,
		MaxAgents: opts.MaxAgents,
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

	// Render initial message
	msgTmpl, err := template.New("message").Parse(plannerInitialMessageTmpl)
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

	initialMessageFile := filepath.Join(planDir, "initial-message.txt")
	mf, err := os.Create(initialMessageFile)
	if err != nil {
		return fmt.Errorf("creating initial message file: %w", err)
	}
	if err := msgTmpl.Execute(mf, msgData); err != nil {
		mf.Close()
		return fmt.Errorf("rendering initial message: %w", err)
	}
	mf.Close()

	// Build claude command args
	relSystemPrompt := filepath.Join(".claude-mux", "plan-"+planID, "system-prompt.txt")
	relInitialMessage := filepath.Join(".claude-mux", "plan-"+planID, "initial-message.txt")

	systemPrompt, err := os.ReadFile(filepath.Join(repoRoot, relSystemPrompt))
	if err != nil {
		return fmt.Errorf("reading system prompt: %w", err)
	}
	initialMessage, err := os.ReadFile(filepath.Join(repoRoot, relInitialMessage))
	if err != nil {
		return fmt.Errorf("reading initial message: %w", err)
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Starting plan session %s — collaborate on your PRD, then say \"execute\" to launch the swarm.\n", planID)

	// Replace current process with claude
	args := []string{
		"claude",
		"--append-system-prompt", string(systemPrompt),
		string(initialMessage),
	}

	return syscall.Exec(claudeBin, args, os.Environ())
}

const plannerSystemPromptTmpl = `You are a project planner helping the user flesh out a task into a fully specified PRD (Product Requirements Document) that will be executed by a swarm of AI coding agents.

## Your Role

You are collaborative and conversational. Your job is to:

1. **Understand the goal**: Ask clarifying questions about what the user wants to build or change. Don't assume — dig into requirements, edge cases, and constraints.

2. **Explore the codebase**: Use your tools to read relevant files, understand the architecture, and identify what needs to change. Reference specific files and code.

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

5. **Save and execute**: When the user approves (says "execute", "ship it", "go", "lgtm", or similar), do the following:
   - Save the final PRD to ` + "`" + `{{.PlanDir}}/prd.md` + "`" + `
   - Then run the swarm command to kick off execution:
     ` + "`" + `claude-mux swarm --task "<one-line summary>" --file {{.PlanDir}}/prd.md{{if .AutoMerge}} --auto-merge{{end}} --max-agents {{.MaxAgents}}` + "`" + `

## Guidelines

- Keep subtasks **independent** to avoid merge conflicts between agents. Each subtask should touch different files where possible.
- Be opinionated — suggest good approaches rather than listing options endlessly.
- If the task is small enough for one agent, say so. Not everything needs a swarm.
- Reference actual code paths and function names from the codebase.
- The PRD should be detailed enough that an AI agent can implement each subtask without further clarification.
`

const plannerInitialMessageTmpl = `{{- if .Task -}}
I'd like to work on this: **{{.Task}}**
{{- if .Context}}

Here's some additional context:

{{.Context}}
{{- end}}

Help me flesh this out into a full plan before we execute.
{{- else -}}
I want to start a new project. Help me figure out what to build and create a plan for it.
{{- end}}
`
