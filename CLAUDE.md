# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build

```bash
make build        # builds bin/claude-mux
make clean        # removes binary
```

No tests yet. No linter configured.

## Architecture

claude-mux is a tmux plugin that discovers active Claude Code sessions across tmux panes, displays them in a Bubble Tea TUI popup, and lets you jump to any session. It also acts as a Claude Code hook handler to capture live session status.

### Two execution modes

The single binary serves two purposes depending on arguments:

- `claude-mux` (no args) — launches the TUI overlay
- `claude-mux hook <event>` — processes a Claude Code hook event from stdin, writes a state file to `~/.cache/claude-mux/<session-id>.json`, then exits

### Package dependency graph

```
cmd/claude-mux/main.go
  ├── internal/hook      (hook subcommand)
  ├── internal/ui        (TUI, depends on internal/session)
  └── internal/tmux      (pane jump after TUI exits)

internal/ui
  ├── internal/session   (DiscoverSessions called on 2s poll)
  └── internal/worktree  (DiscoverWorktrees called on 2s poll)

internal/session
  ├── internal/tmux      (ListPanes, IsClaudePane)
  └── internal/hook      (ReadState for live status enrichment)
```

### Session discovery flow

1. `tmux list-panes -a` with a custom format string using `%%DELIM%%` separator
2. Filter panes where `pane_title` contains "Claude Code" or `pane_current_command` matches semver (e.g., `2.1.42`)
3. Normalize `pane_current_path` by replacing `/` with `-` to find `~/.claude/projects/<normalized>/sessions-index.json`
4. Read the most recent entry from the index for summary, git branch, message count
5. Tail-read the session's JSONL file (last 8KB) for last activity timestamp
6. Infer state from pane title prefix: braille chars (U+2800–U+28FF) = working, `✳` = waiting
7. Enrich with hook state files from `~/.cache/claude-mux/` (ignored if older than 5 minutes)

### Hook state files

Written by `internal/hook/hook.go`, read by `internal/session/discovery.go`. Format:

```json
{"session_id":"uuid","status":"working|waiting|permission","message":"...","tool":"Bash","timestamp":"RFC3339"}
```

Matched to sessions by checking if `<session-id>.jsonl` exists in the project's Claude directory.

### Key data sources

| Data | Location | Read by |
|------|----------|---------|
| Session metadata | `~/.claude/projects/<path>/sessions-index.json` | `session/index.go` |
| Session transcript | `~/.claude/projects/<path>/<id>.jsonl` | `session/jsonl.go`, `hook/transcript.go` |
| Live hook state | `~/.cache/claude-mux/<id>.json` | `session/discovery.go` via `hook.ReadState()` |
| tmux pane info | `tmux list-panes -a` output | `tmux/tmux.go` |

### TUI

Uses Bubble Tea with the Bubbles `list` component. Two tabs: **Sessions** and **Worktrees**, switched with `Tab`.

**Sessions tab**: Polls `session.DiscoverSessions()` every 2 seconds via `tea.Tick`. The `sessionItem` type implements `list.DefaultItem`. On enter, jumps to the selected session's pane. Press `p` to pin/unpin.

**Worktrees tab**: Polls `worktree.DiscoverWorktrees()` on the same 2s tick. Shows all git worktrees discovered from tmux pane paths. Worktrees with active Claude sessions are marked with `*` and can be jumped to with `Enter`. Press `d`/`x` to remove a worktree (confirmation required). Main worktrees cannot be removed. Removing a worktree with an active session shows a warning but is allowed.

Key packages: `internal/ui/tabs.go` (tab bar rendering), `internal/ui/worktree_list.go` (worktree list items), `internal/worktree/worktree.go` (discovery and removal via `git worktree` commands).

### Worktree split keybindings

`scripts/worktree-split.sh` creates a git worktree from the current pane's working directory, opens a tmux split, and launches `claude` in it. Registered in `claude-mux.tmux` with two keybindings:

| Keybind | tmux option | Default | Effect |
|---------|-------------|---------|--------|
| `prefix + T` | `@claude-mux-worktree-h-key` | `T` | Horizontal split (panes stacked) |
| `prefix + t` | `@claude-mux-worktree-v-key` | `t` | Vertical split (panes side by side) |

Worktrees are created as sibling directories named `<repo>-wt-<timestamp>-<id>` with branch `worktree/<timestamp>-<id>`.
