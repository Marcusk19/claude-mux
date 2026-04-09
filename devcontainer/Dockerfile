FROM node:20

ARG CLAUDE_CODE_VERSION=latest

# Install development tools, iptables for firewall, and tmux
RUN apt-get update && apt-get install -y --no-install-recommends \
  less \
  git \
  procps \
  sudo \
  jq \
  curl \
  iptables \
  ipset \
  iproute2 \
  dnsutils \
  aggregate \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Ensure default node user has access to /usr/local/share
RUN mkdir -p /usr/local/share/npm-global && \
  chown -R node:node /usr/local/share/npm-global

# Create workspace directory
RUN mkdir -p /workspace && chown -R node:node /workspace

# Set secure umask globally — new files default to owner-only (600/700)
RUN echo "umask 0077" >> /etc/profile.d/secure-umask.sh && \
  echo "umask 0077" >> /home/node/.bashrc && \
  echo "umask 0077" >> /home/node/.profile

# Create Claude config directory with restricted permissions
RUN mkdir -p /home/node/.claude && \
  chmod 700 /home/node/.claude && \
  chown -R node:node /home/node/.claude

# Set up non-root user
USER node

ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=$PATH:/usr/local/share/npm-global/bin
ENV DEVCONTAINER=true

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}

# Copy firewall, security, and hook scripts
COPY init-firewall.sh hook-handler.sh security-init.sh security-precheck.sh supply-chain-check.sh /usr/local/bin/
USER root
RUN chmod +x /usr/local/bin/init-firewall.sh /usr/local/bin/hook-handler.sh /usr/local/bin/security-init.sh /usr/local/bin/security-precheck.sh /usr/local/bin/supply-chain-check.sh && \
  echo "node ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh" > /etc/sudoers.d/node-firewall && \
  chmod 0440 /etc/sudoers.d/node-firewall

# Install .npmrc with supply chain security defaults (ignore-scripts, audit)
COPY --chown=node:node npmrc /home/node/.npmrc
USER node

WORKDIR /workspace
