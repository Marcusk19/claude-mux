#!/bin/sh
# Lightweight hook handler for sandboxed Claude Code sessions.
# Replaces the Go claude-mux binary for in-container use.
# Reads hook event JSON from stdin and writes state files.
set -e

EVENT="$1"
CACHE_DIR="${CLAUDE_MUX_CACHE:-$HOME/.cache/claude-mux}"
mkdir -p "$CACHE_DIR"

# Read hook event JSON from stdin
INPUT=$(cat)

# Extract session_id from the hook event
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty' 2>/dev/null)
if [ -z "$SESSION_ID" ]; then
  exit 0
fi

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

case "$EVENT" in
  "NotificationReceived")
    # Extract tool and message from the notification
    TOOL=$(echo "$INPUT" | jq -r '.tool_name // "unknown"' 2>/dev/null)
    MESSAGE=$(echo "$INPUT" | jq -r '.message // ""' 2>/dev/null)
    STATUS="working"

    # Write state file
    jq -n \
      --arg sid "$SESSION_ID" \
      --arg status "$STATUS" \
      --arg msg "$MESSAGE" \
      --arg tool "$TOOL" \
      --arg ts "$TIMESTAMP" \
      '{session_id: $sid, status: $status, message: $msg, tool: $tool, timestamp: $ts}' \
      > "$CACHE_DIR/$SESSION_ID.json"
    ;;

  "PostToolUse")
    TOOL=$(echo "$INPUT" | jq -r '.tool_name // "unknown"' 2>/dev/null)
    STATUS="working"

    jq -n \
      --arg sid "$SESSION_ID" \
      --arg status "$STATUS" \
      --arg msg "" \
      --arg tool "$TOOL" \
      --arg ts "$TIMESTAMP" \
      '{session_id: $sid, status: $status, message: $msg, tool: $tool, timestamp: $ts}' \
      > "$CACHE_DIR/$SESSION_ID.json"
    ;;

  "Stop")
    # Write final state
    jq -n \
      --arg sid "$SESSION_ID" \
      --arg status "done" \
      --arg msg "Session completed" \
      --arg tool "" \
      --arg ts "$TIMESTAMP" \
      '{session_id: $sid, status: $status, message: $msg, tool: $tool, timestamp: $ts}' \
      > "$CACHE_DIR/$SESSION_ID.json"

    # Write completion marker for host-side detection
    WORKDIR=$(pwd)
    MARKER="$WORKDIR/.claude-mux/completed"
    if [ -d "$WORKDIR/.claude-mux" ]; then
      echo "$TIMESTAMP" > "$MARKER"
    fi
    ;;
esac
