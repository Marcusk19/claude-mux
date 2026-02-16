# claude-mux

A tmux plugin that shows all active Claude Code sessions in a popup overlay. See what each session is doing, what it's asking, and jump to any of them.

![demo concept](https://img.shields.io/badge/tmux-plugin-blue)

```
┌─────────────── Claude Code Sessions ───────────────┐
│                                                     │
│  ⚡ ~/projects/my-api   main                        │
│     Adding pagination to /users endpoint  [12s ago] │
│                                                     │
│  🔐 ~/projects/web-app   master                     │
│     Claude needs your permission to use Bash [1m]   │
│                                                     │
│  ⏳ ~/projects/infra   feat/monitoring               │
│     Which logging framework should we use?  [5m ago]│
│                                                     │
│  j/k navigate  / filter  enter jump  q quit         │
└─────────────────────────────────────────────────────┘
```

**Status indicators:**
- ⚡ **Working** — Claude is actively processing (running tools, generating response)
- ⏳ **Waiting** — Claude finished and is waiting for your input
- 🔐 **Permission** — Claude needs permission to proceed
- ❓ **Unknown** — Claude Code detected but state couldn't be determined

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

## Live status via Claude Code hooks

By default, the session list shows the static summary from Claude's session index (e.g., "Fix Cypress Test"). To get **live status** — what Claude is currently doing, what tool it's running, or what question it's asking — you need to configure Claude Code hooks.

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

| Hook event | Status shown | What it captures |
|---|---|---|
| `UserPromptSubmit` | ⚡ Working | Your prompt text (truncated) |
| `PreToolUse` | ⚡ Working | Tool description — e.g., "Reading ~/file.go", "$ npm test", "Searching: pattern" |
| `Stop` | ⏳ Waiting | Claude's last message, extracted from the session transcript (prioritizes questions) |
| `Notification` | 🔐 Permission / ⏳ Waiting | The notification message (e.g., "Claude needs your permission to use Bash") |

### How it works

Each hook invocation writes a small JSON state file to `~/.cache/claude-mux/<session-id>.json`. The TUI reads these files during its polling loop and shows the live status instead of the static session summary. State files older than 5 minutes are ignored.

## How session discovery works

Claude Code sessions are detected without `ps` — two signals are used:

1. **`tmux list-panes -a`** — panes where `pane_title` contains `"Claude Code"` (e.g., `"✳ Claude Code"`, `"⠐ Claude Code"`)
2. **Fallback:** `pane_current_command` matches a semver pattern (e.g., `2.1.42`) since Claude sets its version as the process name

For each detected pane, the working directory is normalized and matched to `~/.claude/projects/<normalized-path>/sessions-index.json` to pull summary, git branch, and message count.
