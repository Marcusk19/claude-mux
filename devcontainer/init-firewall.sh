#!/usr/bin/env bash
# Sandbox firewall: default-deny outbound, whitelist only essential services.
# Adapted from the Anthropic Claude Code reference devcontainer.
set -euo pipefail

# Regex patterns for validation
CIDR_REGEX='^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$'
IP_REGEX='^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'

echo "==> Initializing sandbox firewall..."

# Preserve Docker DNS rules before flushing
DOCKER_DNS_RULES=$(iptables-save | grep -i "docker" || true)

# Flush existing rules
iptables -F
iptables -X 2>/dev/null || true
iptables -t nat -F
iptables -t nat -X 2>/dev/null || true

# Restore Docker DNS rules if they existed
if [ -n "$DOCKER_DNS_RULES" ]; then
  echo "$DOCKER_DNS_RULES" | iptables-restore --noflush 2>/dev/null || true
fi

# Allow loopback
iptables -A INPUT -i lo -j ACCEPT
iptables -A OUTPUT -o lo -j ACCEPT

# Allow established/related connections
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Allow DNS (needed to resolve whitelisted domains)
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT

# Allow SSH
iptables -A OUTPUT -p tcp --dport 22 -j ACCEPT

# Detect host network (Docker gateway) and allow it
HOST_IP=$(ip route | grep default | awk '{print $3}' || true)
if [ -n "$HOST_IP" ]; then
  iptables -A OUTPUT -d "$HOST_IP" -j ACCEPT
  echo "    Allowed host gateway: $HOST_IP"
fi

# Create ipset for allowed domains
ipset create allowed-domains hash:net -exist
ipset flush allowed-domains

# Function to add resolved IPs to the allowed set
add_domain() {
  local domain="$1"
  local ips
  ips=$(dig +short "$domain" A 2>/dev/null || true)
  for ip in $ips; do
    if [[ "$ip" =~ $IP_REGEX ]]; then
      ipset add allowed-domains "$ip/32" -exist
    fi
  done
}

# Function to add a CIDR block to the allowed set
add_cidr() {
  local cidr="$1"
  if [[ "$cidr" =~ $CIDR_REGEX ]]; then
    ipset add allowed-domains "$cidr" -exist
  fi
}

# --- Whitelist: npm registry ---
echo "    Resolving npm registry..."
add_domain "registry.npmjs.org"
add_domain "registry.yarnpkg.com"

# --- Whitelist: GitHub ---
echo "    Fetching GitHub IP ranges..."
GH_META=$(curl -sf https://api.github.com/meta 2>/dev/null || true)
if [ -n "$GH_META" ]; then
  for key in web api git; do
    for cidr in $(echo "$GH_META" | jq -r ".${key}[]?" 2>/dev/null); do
      add_cidr "$cidr"
    done
  done
else
  # Fallback: resolve GitHub domains directly
  for domain in github.com api.github.com; do
    add_domain "$domain"
  done
fi

# --- Whitelist: Anthropic API ---
echo "    Resolving Anthropic services..."
add_domain "api.anthropic.com"
add_domain "statsig.anthropic.com"
add_domain "sentry.io"

# --- Whitelist: Google Cloud / Vertex AI ---
echo "    Resolving Google Cloud (Vertex AI)..."
add_domain "oauth2.googleapis.com"
add_domain "accounts.google.com"
# Vertex AI regional endpoints (us-east5, europe-west1, etc.)
for region in us-east5 us-central1 europe-west1 europe-west4 asia-southeast1; do
  add_domain "${region}-aiplatform.googleapis.com"
done
add_domain "aiplatform.googleapis.com"

# --- Whitelist: PyPI (for Python projects) ---
echo "    Resolving PyPI..."
add_domain "pypi.org"
add_domain "files.pythonhosted.org"

# Allow traffic to whitelisted IPs
iptables -A OUTPUT -m set --match-set allowed-domains dst -j ACCEPT

# Default deny all other outbound traffic
iptables -A OUTPUT -j DROP
iptables -P OUTPUT DROP

echo "==> Firewall configured. Verifying..."

# Verify: blocked site should fail
if curl -sf --connect-timeout 3 https://example.com >/dev/null 2>&1; then
  echo "    WARNING: example.com is reachable (firewall may not be working)"
else
  echo "    OK: example.com blocked"
fi

# Verify: npm registry should work
if curl -sf --connect-timeout 5 https://registry.npmjs.org/ >/dev/null 2>&1; then
  echo "    OK: npm registry reachable"
else
  echo "    WARNING: npm registry unreachable (may need to refresh DNS)"
fi

echo "==> Firewall ready."
