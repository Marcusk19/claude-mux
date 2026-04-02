package devcontainer

import "embed"

//go:embed Dockerfile init-firewall.sh hook-handler.sh
var Assets embed.FS
