# claude-mux

A tmux plugin that shows all active Claude Code sessions in a popup overlay. See what each session is doing, what it's asking, and jump to any of them.

![demo concept](https://img.shields.io/badge/tmux-plugin-blue)

```
┌─────────────── Claude Code Sessions ───────────────┐
│                                                     │
│  ⏳ ~/projects/my-api   main              (cyan)    │
│     add pagination to the /users endpoint [12s ago] │
│                                                     │
│  🔐 ~/projects/web-app   master          (orange)   │
│     fix the auth redirect loop on /login  [1m ago]  │
│                                                     │
│  🔒 ~/projects/infra   feat/monitoring   (orange)   │
│     set up prometheus monitoring stack    [5m ago]   │
│                                                     │
│  j/k navigate  / filter  enter jump  q quit         │
└─────────────────────────────────────────────────────┘
```

Session paths are **color-coded by state** so you can scan status at a glance:

- ⏳ **Cyan** — Claude is actively processing (running tools, generating response)
- 🔒 **Orange** — Claude finished and is waiting for your input
- 🔐 **Orange** — Claude needs permission to proceed
- ❓ **Gray** — Claude Code detected but state couldn't be determined

The description line shows a **truncated snippet of the initial user prompt** — what you first asked Claude in each session. Falls back to the session summary if the prompt can't be read.

## Requirements

- tmux >= 3.2 (for `display-popup`; falls back to `new-window` on older versions)
- Go 1.24+ (for building from source)

## Installation

### With TPM (recommended)

Add to your `~/.tmux.conf`:

```tmux
set -g @plugin 'Marcusk19/claude-mux'
```

Then press `prefix + I` to install.

### Manual

```bash
git clone https://github.com/Marcusk19/claude-mux.git ~/.tmux/plugins/claude-mux
cd ~/.tmux/plugins/claude-mux
make build
```

Add to your `~/.tmux.conf`:

```tmux
run-shell '~/.tmux/plugins/claude-mux/claude-mux.tmux'
```

Reload tmux:

```bash
tmux source-file ~/.tmux.conf
```

## Usage

Press `prefix + C` to open the session overlay.

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `Enter` | Jump to selected session |
| `/` | Filter by path, summary, or branch |
| `q` / `Esc` | Close popup |

The list auto-refreshes every 2 seconds.

## Configuration

Optional tmux options (set before the plugin loads):

```tmux
set -g @claude-mux-key 'C'        # Keybinding (default: C, so prefix + C)
set -g @claude-mux-width '80%'    # Popup width (default: 80%)
set -g @claude-mux-height '70%'   # Popup height (default: 70%)
```

### Notification sound

When hooks are configured, claude-mux plays a sound when Claude needs your attention (`Stop`, `Notification`, and permission prompt events). This uses `afplay` on macOS.

| `CLAUDE_MUX_SOUND` | Behavior |
|---|---|
| *(unset)* | Plays `/System/Library/Sounds/Funk.aiff` |
| Path to a sound file | Plays that file instead |
| `0` | Disables sound |

```bash
# Use a different sound
export CLAUDE_MUX_SOUND="/System/Library/Sounds/Glass.aiff"

# Disable sound
export CLAUDE_MUX_SOUND=0
```

## Agent orchestration

claude-mux includes CLI subcommands for orchestrating multiple Claude Code agents. An orchestrator Claude Code session can spawn subagents in separate tmux panes, each working in an isolated git worktree. No MCP server or API keys needed — subcommands are called via Bash.

### Subcommands

#### `claude-mux spawn`

Creates a git worktree, writes a task file, and opens a new tmux pane with an interactive Claude Code session.

```bash
claude-mux spawn --task "Add input validation to the API" [--context "Use zod for schema validation"] [--file src/api.ts]
```

| Flag | Description |
|------|-------------|
| `--task` | Task description (required) |
| `--context` | Additional context to include in the task file |
| `--file` | Comma-separated file paths to embed in the task file |

Prints the task ID to stdout. The subagent runs interactively with `--dangerously-skip-permissions` so it can work autonomously.

#### `claude-mux status`

Shows the current state of all subagents for the current orchestrator.

```bash
claude-mux status
```

```
[20260323-143052-a1b2c3] running  (worktree/20260323-143052-a1b2c3, 45s ago)
  Task: Add input validation to the API
  Tool: Edit
  Pane: %42  Worktree: ~/projects/my-api-wt-20260323-143052-a1b2c3
```

Status is detected automatically: `running` if the tmux pane exists, `completed` if the pane is gone and the branch has commits, `failed` if the pane is gone with no commits. Live tool activity is enriched from hook state files.

#### `claude-mux collect`

Gathers results (commits and diff stats) from completed subagents.

```bash
claude-mux collect [--task-id ID] [--merge] [--cleanup]
```

| Flag | Description |
|------|-------------|
| `--task-id` | Collect a specific subagent's results |
| `--merge` | Merge completed branches into the current branch |
| `--cleanup` | Remove worktrees and branches after collecting |

#### `claude-mux cleanup`

Removes worktrees, branches, and state files for completed subagents.

```bash
claude-mux cleanup [--task-id ID] [--force]
```

| Flag | Description |
|------|-------------|
| `--task-id` | Clean up a specific subagent |
| `--force` | Also remove running subagents |

### Orchestrator identity

Subagents are grouped by orchestrator ID, resolved in order:

1. `CLAUDE_SESSION_ID` environment variable (set automatically by Claude Code)
2. `.claude-mux/orchestrator-id` file in the repo root
3. Auto-generated and persisted to the file above

### Workflow example

From an orchestrator Claude Code session:

```bash
# Spawn parallel subagents
claude-mux spawn --task "Add user authentication with JWT tokens"
claude-mux spawn --task "Add rate limiting middleware"
claude-mux spawn --task "Add request logging with structured output"

# Monitor progress
claude-mux status

# Collect results when done
claude-mux collect

# Merge and clean up
claude-mux collect --merge --cleanup
```

## State detection via Claude Code hooks

By default, session state is inferred from the pane title (braille characters = working, `✳` = waiting). For **more accurate state detection** — distinguishing permission prompts from regular waiting, and faster state transitions — configure Claude Code hooks.

Add the following to your `~/.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-mux/bin/claude-mux hook Stop",
            "timeout": 5
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-mux/bin/claude-mux hook Notification",
            "timeout": 5
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-mux/bin/claude-mux hook PreToolUse",
            "timeout": 5
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-mux/bin/claude-mux hook UserPromptSubmit",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

Replace `/path/to/claude-mux` with the actual install path (e.g., `~/.tmux/plugins/claude-mux`).

### What each hook captures

| Hook event | State set |
|---|---|
| `UserPromptSubmit` | ⏳ Working (cyan) |
| `PreToolUse` | ⏳ Working (cyan) |
| `Stop` | 🔒 Waiting (orange) |
| `Notification` | 🔐 Permission (orange) or 🔒 Waiting (orange) |

### How it works

Each hook invocation writes a small JSON state file to `~/.cache/claude-mux/<session-id>.json`. The TUI reads these files during its polling loop to determine the session's activity state for color-coding. State files older than 5 minutes are ignored.

## How session discovery works

Claude Code sessions are detected without `ps` — two signals are used:

1. **`tmux list-panes -a`** — panes where `pane_title` contains `"Claude Code"` (e.g., `"✳ Claude Code"`, `"⠐ Claude Code"`)
2. **Fallback:** `pane_current_command` matches a semver pattern (e.g., `2.1.42`) since Claude sets its version as the process name

For each detected pane, the working directory is normalized and matched to `~/.claude/projects/<normalized-path>/sessions-index.json` to pull summary, git branch, and message count.
