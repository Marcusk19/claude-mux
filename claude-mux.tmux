#!/usr/bin/env bash

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$CURRENT_DIR/bin/claude-mux"

# Build if binary doesn't exist
if [ ! -f "$BINARY" ]; then
    cd "$CURRENT_DIR" && make build
fi

# Read user-configurable options
key=$(tmux show-option -gqv @claude-mux-key)
key=${key:-C}

popup_width=$(tmux show-option -gqv @claude-mux-width)
popup_width=${popup_width:-80%}

popup_height=$(tmux show-option -gqv @claude-mux-height)
popup_height=${popup_height:-70%}

# Check tmux version for display-popup support (>= 3.2)
tmux_version=$(tmux -V | sed -En 's/^tmux ([0-9]+\.[0-9]+).*/\1/p')
has_popup=$(echo "$tmux_version >= 3.2" | bc 2>/dev/null || echo 0)

if [ "$has_popup" = "1" ]; then
    tmux bind-key "$key" display-popup -E -w "$popup_width" -h "$popup_height" "'$BINARY'"
else
    tmux bind-key "$key" new-window "'$BINARY'"
fi

# Worktree split keybindings
worktree_h_key=$(tmux show-option -gqv @claude-mux-worktree-h-key)
worktree_h_key=${worktree_h_key:-T}

worktree_v_key=$(tmux show-option -gqv @claude-mux-worktree-v-key)
worktree_v_key=${worktree_v_key:-t}

WORKTREE_SCRIPT="$CURRENT_DIR/scripts/worktree-split.sh"

# -v = horizontal split (panes stacked), -h = vertical split (panes side by side)
tmux bind-key "$worktree_h_key" run-shell "'$WORKTREE_SCRIPT' -v '#{pane_current_path}'"
tmux bind-key "$worktree_v_key" run-shell "'$WORKTREE_SCRIPT' -h '#{pane_current_path}'"
