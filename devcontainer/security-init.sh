#!/bin/sh
# Runtime permission hardening for sandboxed Claude Code sessions.
# Called from the container entry script before launching claude.
# Fixes world-writable files created by volume mounts or incorrect umask.
set -e

echo "[Security] Hardening file permissions..."

# Credentials — owner-only read/write
if [ -f "$HOME/.claude/.credentials.json" ]; then
    chmod 600 "$HOME/.claude/.credentials.json"
fi

# Settings files — owner-only (may contain permission overrides)
find /workspace/.claude -type f -name "settings.local.json" -exec chmod 600 {} \; 2>/dev/null || true
find "$HOME/.claude" -type f -name "settings.local.json" -exec chmod 600 {} \; 2>/dev/null || true

# Agent and command definitions — readable but not writable by others
find "$HOME/.claude/agents" -type f -exec chmod 644 {} \; 2>/dev/null || true
find "$HOME/.claude/commands" -type f -exec chmod 644 {} \; 2>/dev/null || true

# Backups contain sensitive config — owner-only
find "$HOME/.claude/backups" -type f -exec chmod 600 {} \; 2>/dev/null || true

# Sensitive directories — owner-only access
for dir in projects shell-snapshots session-env; do
    chmod 700 "$HOME/.claude/$dir" 2>/dev/null || true
done

# Workspace .claude directory
if [ -d /workspace/.claude ]; then
    find /workspace/.claude -type f -exec chmod 644 {} \;
    find /workspace/.claude -type d -exec chmod 755 {} \;
    chmod 600 /workspace/.claude/settings.local.json 2>/dev/null || true
fi

echo "[Security] Permission hardening complete"
