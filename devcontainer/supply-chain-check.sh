#!/bin/bash
#
# Supply Chain Security Check
# Validates package installations for security risks
#
# Usage: ./supply-chain-check.sh npm|pip|go <package-spec>
# Exit: 0 = safe, 1 = blocked
#

set -euo pipefail

PACKAGE_MANAGER="${1:-}"
PACKAGE_SPEC="${2:-}"

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

warn() {
    local warning="$1"
    echo "{\"decision\":\"allow\",\"warning\":\"$warning\"}"
    exit 0
}

# ============================================================================
# NPM/BUN Supply Chain Checks
# ============================================================================

if [[ "$PACKAGE_MANAGER" == "npm" ]] || [[ "$PACKAGE_MANAGER" == "bun" ]]; then
    
    # Check for git+ protocol (unverified source)
    if echo "$PACKAGE_SPEC" | grep -qE 'git\+|github:|gitlab:'; then
        block "Installing from git URL bypasses npm registry security. Clone and review the repository first, then install locally."
    fi
    
    # Check for http:// (insecure)
    if echo "$PACKAGE_SPEC" | grep -qE 'http://'; then
        block "Insecure HTTP package source detected. Use HTTPS only."
    fi
    
    # Check for file:// protocol (could be trojaned local file)
    if echo "$PACKAGE_SPEC" | grep -qE 'file://'; then
        warn "Installing from local file path. Ensure the package source is trusted."
    fi
    
    # Check for --ignore-scripts bypass
    if echo "$PACKAGE_SPEC" | grep -qE -- '--ignore-scripts|--no-scripts'; then
        warn "Lifecycle scripts disabled. This bypasses package security checks but prevents malicious script execution."
    fi
    
    # Check for suspicious package names (typosquatting patterns)
    SUSPICIOUS_PATTERNS=(
        'eIpress'  # express typo
        'reacl'    # react typo
        'requesf'  # request typo
        'cross-env.js'  # known typosquat
        'babelcli'      # babel-cli typo
        'crossenv'      # cross-env typo
    )
    
    for pattern in "${SUSPICIOUS_PATTERNS[@]}"; do
        if echo "$PACKAGE_SPEC" | grep -qiE "$pattern"; then
            block "Potential typosquatting detected: Package name matches known malicious pattern '$pattern'. Verify package name carefully."
        fi
    done
    
    # Check for single-character packages (often malicious)
    if echo "$PACKAGE_SPEC" | grep -qE '^[a-z]$|^[a-z][0-9]$'; then
        warn "Single or two-character package name detected. Verify this is the correct package."
    fi
    
    allow
fi

# ============================================================================
# Python/Pip Supply Chain Checks
# ============================================================================

if [[ "$PACKAGE_MANAGER" == "pip" ]] || [[ "$PACKAGE_MANAGER" == "python" ]]; then
    
    # Check for --extra-index-url with non-PyPI domains
    if echo "$PACKAGE_SPEC" | grep -qE -- '--extra-index-url|--index-url|-i'; then
        if ! echo "$PACKAGE_SPEC" | grep -qE 'pypi\.org|python\.org|pythonhosted\.org'; then
            block "Untrusted Python package index detected. Only use official PyPI (pypi.org) or verified mirrors."
        fi
    fi
    
    # Check for --trusted-host (disables SSL verification)
    if echo "$PACKAGE_SPEC" | grep -qE -- '--trusted-host'; then
        block "SSL verification bypass detected (--trusted-host). This allows man-in-the-middle attacks."
    fi
    
    # Check for git+ URLs
    if echo "$PACKAGE_SPEC" | grep -qE 'git\+'; then
        block "Installing from git URL bypasses PyPI security scanning. Clone and review first."
    fi
    
    # Check for http:// (insecure)
    if echo "$PACKAGE_SPEC" | grep -qE 'http://'; then
        block "Insecure HTTP package source detected. Use HTTPS only."
    fi
    
    # Check for suspicious package names (case-insensitive)
    PY_SUSPICIOUS_PATTERNS=(
        'urlib'        # urllib typo
        'pip-install'  # suspicious
        'setup-tools'  # setuptools typo
    )

    for pattern in "${PY_SUSPICIOUS_PATTERNS[@]}"; do
        if echo "$PACKAGE_SPEC" | grep -qiE "$pattern"; then
            block "Potential typosquatting detected: Package name matches known malicious pattern '$pattern'."
        fi
    done

    # Case-sensitive: reQuests (capital Q distinguishes from legitimate 'requests')
    if echo "$PACKAGE_SPEC" | grep -qE 'reQuests'; then
        block "Potential typosquatting detected: Package name matches known malicious pattern 'reQuests'."
    fi
    
    allow
fi

# ============================================================================
# Go Module Supply Chain Checks
# ============================================================================

if [[ "$PACKAGE_MANAGER" == "go" ]]; then
    
    # Check for replace directives pointing to local paths (could be trojaned)
    if echo "$PACKAGE_SPEC" | grep -qE 'replace.*=>.*file://|replace.*=>.*\.\.\/'; then
        warn "Go module replace directive with local path detected. Ensure replacement module is trusted."
    fi
    
    # Check for insecure git URLs
    if echo "$PACKAGE_SPEC" | grep -qE 'git://|http://'; then
        block "Insecure protocol detected in Go module path. Use HTTPS only."
    fi
    
    allow
fi

# Unknown package manager
warn "Unknown package manager: $PACKAGE_MANAGER. No supply chain checks available."
