package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
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
// and manages subagents. Returns the swarm ID.
func Swarm(opts SwarmOpts) (string, error) {
	orchID, err := resolveOrchestratorID()
	if err != nil {
		return "", fmt.Errorf("resolving orchestrator ID: %w", err)
	}

	repoRoot, err := gitRepoRoot(".")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	swarmID := generateTaskID()

	if opts.MaxAgents <= 0 {
		opts.MaxAgents = 3
	}

	// Create swarm directory
	swarmDir := filepath.Join(repoRoot, ".claude-mux", "swarm-"+swarmID)
	if err := os.MkdirAll(swarmDir, 0o755); err != nil {
		return "", fmt.Errorf("creating swarm directory: %w", err)
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
			return "", fmt.Errorf("reading %s: %w", f, err)
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

	// Render coordinator prompt
	tmpl, err := template.New("coordinator").Parse(coordinatorPromptTmpl)
	if err != nil {
		return "", fmt.Errorf("parsing coordinator template: %w", err)
	}

	promptData := struct {
		Task      string
		Context   string
		AutoMerge bool
		MaxAgents int
	}{
		Task:      opts.Task,
		Context:   contextBuf.String(),
		AutoMerge: opts.AutoMerge,
		MaxAgents: opts.MaxAgents,
	}

	promptFile := filepath.Join(swarmDir, "prompt.txt")
	f, err := os.Create(promptFile)
	if err != nil {
		return "", fmt.Errorf("creating prompt file: %w", err)
	}
	if err := tmpl.Execute(f, promptData); err != nil {
		f.Close()
		return "", fmt.Errorf("rendering coordinator prompt: %w", err)
	}
	f.Close()

	// Launch coordinator Claude session in a tmux split in the main repo dir
	shellCmd := fmt.Sprintf(`claude --dangerously-skip-permissions "$(cat %s)"`,
		filepath.Join(".claude-mux", "swarm-"+swarmID, "prompt.txt"))

	out, err := exec.Command(
		"tmux", "split-window", "-v",
		"-c", repoRoot,
		"-P", "-F", "#{pane_id}",
		shellCmd,
	).Output()
	if err != nil {
		return "", fmt.Errorf("opening tmux pane: %w", err)
	}
	paneID := strings.TrimSpace(string(out))

	// Write orchestrator-id so the coordinator resolves to the same orchestrator
	orchIDFile := filepath.Join(repoRoot, ".claude-mux", "orchestrator-id")
	os.WriteFile(orchIDFile, []byte(orchID+"\n"), 0o644)

	fmt.Fprintf(os.Stderr, "Swarm %s launched in pane %s (orchestrator: %s)\n", swarmID, paneID, orchID)
	return swarmID, nil
}

const coordinatorPromptTmpl = `You are a swarm coordinator. Your job is to break down a task into independent subtasks, spawn subagents to implement them, validate results, and collect everything when done.

## Task

{{.Task}}
{{- if .Context}}

## Context

{{.Context}}
{{- end}}

## Instructions

1. **Analyze the task** and break it into 2-6 independent subtasks that can be worked on in parallel. Each subtask should be a self-contained unit of work.

2. **Spawn implementers** for each subtask using:
   ` + "`" + `claude-mux spawn --task "implement: <detailed subtask description>"` + "`" + `

   Spawn at most {{.MaxAgents}} agents at a time. If you have more subtasks than that, wait for some to complete before spawning more.

3. **Monitor progress** by running:
   ` + "`" + `claude-mux status` + "`" + `

   Poll every 30 seconds until agents complete. An agent is "completed" when its tmux pane is gone and it has commits on its branch. An agent is "failed" if its pane is gone with no new commits.

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

## Rules

- Do NOT modify code directly. Your only job is to coordinate subagents.
- Keep subtasks focused and independent to minimize merge conflicts.
- If a subtask fails, you may retry it once with a refined task description.
- If validation fails, spawn a new implementer to fix the issues in the same worktree.
`
