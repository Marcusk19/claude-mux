package devcontainer

import "embed"

//go:embed Dockerfile init-firewall.sh hook-handler.sh security-init.sh security-precheck.sh supply-chain-check.sh npmrc
var Assets embed.FS
