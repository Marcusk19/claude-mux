#!/usr/bin/env bash
set -euo pipefail

SPLIT_FLAG="$1"   # -h (vertical split) or -v (horizontal split)
PANE_PATH="$2"    # #{pane_current_path} from tmux

cd "$PANE_PATH"

# Must be in a git repo
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    tmux display-message "claude-mux: not a git repository"
    exit 1
fi

repo_root=$(git rev-parse --show-toplevel)
repo_name=$(basename "$repo_root")

# Auto-generate branch name and worktree path
timestamp=$(date +%Y%m%d-%H%M%S)
short_id=$(head -c 4 /dev/urandom | xxd -p | head -c 6)
branch_name="worktree/${timestamp}-${short_id}"
worktree_dir="${repo_root}/../${repo_name}-wt-${timestamp}-${short_id}"

if ! git worktree add "$worktree_dir" -b "$branch_name" 2>/dev/null; then
    tmux display-message "claude-mux: failed to create worktree"
    exit 1
fi

worktree_dir=$(cd "$worktree_dir" && pwd)

tmux split-window "$SPLIT_FLAG" -c "$worktree_dir" "claude"
