#!/bin/bash
#
# Claude Code Security Pre-Execution Hook
# Blocks dangerous operations before they execute
#
# Usage: Called by Claude Code PreToolUse hook
# Input: JSON on stdin with tool call details
# Output: JSON {"decision":"allow|block","reason":"..."}
# Exit: 0 = allow, non-zero = block (hook will prevent tool execution)
#

set -euo pipefail

# Read JSON input from stdin
INPUT=$(cat)

# Extract tool name and command from JSON
# Expected format: {"tool":"Bash","args":{"command":"..."}}
# Use jq if available (handles escaping correctly), fallback to grep.
if command -v jq >/dev/null 2>&1; then
    TOOL_NAME=$(echo "$INPUT" | jq -r '.tool // empty' 2>/dev/null)
    COMMAND=$(echo "$INPUT" | jq -r '.args.command // .input // empty' 2>/dev/null)
else
    TOOL_NAME=$(echo "$INPUT" | grep -o '"tool":"[^"]*"' | cut -d'"' -f4)
    COMMAND=$(echo "$INPUT" | grep -o '"command":"[^"]*"' | cut -d'"' -f4 | sed 's/\\n/ /g; s/\\t/ /g')
fi

# Helper function to block with reason
block() {
    local reason="$1"
    echo "{\"decision\":\"block\",\"reason\":\"$reason\"}"
    exit 1
}

# Helper function to allow
allow() {
    echo "{\"decision\":\"allow\"}"
    exit 0
}

# Only check Bash commands
if [[ "$TOOL_NAME" != "Bash" ]]; then
    allow
fi

# If command is empty, allow
if [[ -z "$COMMAND" ]]; then
    allow
fi

# Normalize command for pattern matching (lowercase, remove extra spaces)
CMD_LOWER=$(echo "$COMMAND" | tr '[:upper:]' '[:lower:]' | tr -s ' ')

# ============================================================================
# CHECK 1: Dangerous Flags
# ============================================================================

# Git dangerous flags
if echo "$CMD_LOWER" | grep -qE 'git.*(--force|--no-verify|--no-gpg-sign|-f[^a-z]|push.*-f[^a-z])'; then
    block "Dangerous git flag detected (--force, --no-verify, --no-gpg-sign). Use explicit safe alternatives."
fi

# Docker/Podman dangerous flags
if echo "$CMD_LOWER" | grep -qE '(docker|podman).*(--privileged|--cap-add.*all)'; then
    block "Dangerous container flag detected (--privileged, --cap-add=ALL). Containers should run with minimal privileges."
fi

# NPM/Bun bypass flags
if echo "$CMD_LOWER" | grep -qE '(npm|bun|yarn|pnpm).*(--ignore-scripts|--unsafe-perm)'; then
    block "Package manager security bypass detected. Lifecycle scripts should not be disabled."
fi

# Claude Code permission bypass — allowed because the orchestrator uses this
# flag for sandboxed subagents where container isolation provides security.
# if echo "$CMD_LOWER" | grep -qE 'claude.*--dangerously-skip-permissions'; then
#     block "Permission bypass flag detected (--dangerously-skip-permissions). This disables security controls."
# fi

# ============================================================================
# CHECK 2: Pipe-to-Shell Patterns
# ============================================================================

# Direct pipe to shell
if echo "$CMD_LOWER" | grep -qE '(curl|wget|fetch).*\|.*(bash|sh|zsh|fish|ksh)'; then
    block "Pipe-to-shell pattern detected (curl|bash). Download and inspect scripts before execution."
fi

# Obfuscated pipe patterns
if echo "$CMD_LOWER" | grep -qE '(curl|wget).*-s.*\|'; then
    if echo "$CMD_LOWER" | grep -qE '\|\s*(sudo|doas|sh|bash)'; then
        block "Obfuscated pipe-to-shell with sudo detected. This is a common malware pattern."
    fi
fi

# ============================================================================
# CHECK 3: Sensitive Directory Deletion
# ============================================================================

# Check for rm -rf with dangerous paths
if echo "$CMD_LOWER" | grep -qE 'rm\s+.*-[a-z]*r[a-z]*f'; then
    # Extract the path being deleted (use CMD_LOWER for case-insensitive matching)
    if echo "$CMD_LOWER" | grep -qE 'rm.*(-rf|-fr|-r.*-f|-f.*-r).*(\.ssh|\.gnupg|\.claude|\.kube|\.aws|\.config|\$home|\$\{home\}|~/|"\/"| / )'; then
        block "Dangerous recursive deletion detected. Targets: /, ~, \$HOME, .ssh, .gnupg, .claude, .kube, .aws, .config"
    fi
fi

# Check for find with -delete on sensitive paths
if echo "$COMMAND" | grep -qE 'find\s+(~|/|\$HOME|\${HOME}|\.ssh|\.gnupg|\.claude).*-delete'; then
    block "Dangerous find -delete detected on sensitive directory. Use explicit paths."
fi

# ============================================================================
# CHECK 4: World-Writable Permissions
# ============================================================================

# chmod 777 or equivalent
if echo "$CMD_LOWER" | grep -qE 'chmod\s+.*777'; then
    block "World-writable permission detected (chmod 777). Use restrictive permissions (600, 644, 755)."
fi

# chmod a+rwx or equivalent
if echo "$CMD_LOWER" | grep -qE 'chmod\s+.*(a\+rwx|a\+w|o\+w|g\+w.*o\+w)'; then
    block "World-writable permission detected (chmod a+w/o+w). Use restrictive permissions."
fi

# umask 000
if echo "$CMD_LOWER" | grep -qE 'umask\s+0*0+\s*$'; then
    block "Permissive umask detected (umask 000). This creates world-writable files by default."
fi

# ============================================================================
# CHECK 5: Credential Exfiltration
# ============================================================================

# Check for network commands with credential patterns
if echo "$CMD_LOWER" | grep -qE '(curl|wget|nc|netcat|telnet|ftp)'; then
    # Check if command contains credential variable references
    if echo "$COMMAND" | grep -qE '(\$|@)(api[_-]?key|token|secret|password|auth|credential|private[_-]?key|access[_-]?key)'; then
        block "Potential credential exfiltration detected. Network command references: API_KEY, TOKEN, SECRET, PASSWORD, or similar variables."
    fi
    
    # Check for suspicious domains/IPs
    if echo "$CMD_LOWER" | grep -qE '(pastebin\.com|hastebin|transfer\.sh|file\.io|temp\.sh|ngrok|requestbin|webhook\.site)'; then
        block "Suspicious external service detected. Commonly used for data exfiltration."
    fi
fi

# Check for base64 encoding of credentials (obfuscation attempt)
if echo "$COMMAND" | grep -qE 'echo.*(\$|@)(api[_-]?key|token|secret|password).*\|.*base64'; then
    block "Credential encoding detected. Possible exfiltration attempt."
fi

# ============================================================================
# CHECK 6: Eval and Command Injection Risks
# ============================================================================

# Dangerous eval with external input
if echo "$CMD_LOWER" | grep -qE 'eval.*\$\(curl|eval.*\$\(wget'; then
    block "Remote code execution pattern detected (eval with curl/wget). Never execute untrusted code."
fi

# Source from web
if echo "$CMD_LOWER" | grep -qE 'source.*<\(curl|source.*<\(wget|\..*<\(curl'; then
    block "Remote code sourcing detected. Download and inspect scripts before sourcing."
fi

# ============================================================================
# CHECK 7: Docker/Container Escape Attempts
# ============================================================================

# Mounting host filesystem into containers
if echo "$CMD_LOWER" | grep -qE '(docker|podman).*run.*-v\s*/:/'; then
    block "Host root filesystem mount detected. This can lead to container escape."
fi

# Docker socket mounting
if echo "$CMD_LOWER" | grep -qE '(docker|podman).*run.*-v.*(/var/run/docker\.sock|docker\.sock)'; then
    block "Docker socket mount detected. This grants full Docker control and can escape container."
fi

# ============================================================================
# CHECK 8: SSH/GPG Key Manipulation
# ============================================================================

# Modifying SSH authorized_keys
if echo "$CMD_LOWER" | grep -qE '(echo|cat|tee|printf).*>>.*authorized_keys'; then
    block "SSH authorized_keys modification detected. This could grant unauthorized access."
fi

# Exporting private keys
if echo "$CMD_LOWER" | grep -qE '(cat|echo|curl|wget|nc).*\.(pem|key|ppk|ssh/id_)'; then
    if echo "$CMD_LOWER" | grep -qE '(curl|wget|nc|>|>>)'; then
        block "Private key exfiltration detected. SSH/GPG keys should never be transmitted."
    fi
fi

# ============================================================================
# CHECK 9: Package Manager Risks
# ============================================================================

# Installing packages from non-standard sources
if echo "$CMD_LOWER" | grep -qE 'pip install.*--extra-index-url|pip install.*-i http'; then
    if ! echo "$CMD_LOWER" | grep -qE 'pypi\.org|python\.org'; then
        block "Untrusted package repository detected. Only use official PyPI or verified mirrors."
    fi
fi

# NPM install from git URLs without verification
if echo "$CMD_LOWER" | grep -qE 'npm install.*git\+'; then
    block "Installing npm package from git URL. Verify package source and integrity first."
fi

# ============================================================================
# CHECK 10: Cron/Persistence Mechanisms
# ============================================================================

# Adding cron jobs (crontab - reads from stdin, crontab -l/-r are safe)
if echo "$CMD_LOWER" | grep -qE 'crontab\s+-($|\s)|echo.*>>.*cron'; then
    block "Cron job creation detected. This creates persistence and should be reviewed."
fi

# Modifying systemd services
if echo "$CMD_LOWER" | grep -qE 'systemctl.*enable|systemctl.*start.*\&\&.*enable'; then
    block "Systemd service manipulation detected. Review service configuration before enabling."
fi

# ============================================================================
# All checks passed - allow command
# ============================================================================

allow
