package container

import (
	"fmt"
	"os/exec"
)

// Runtime represents the container runtime (docker or podman).
type Runtime string

const (
	Docker Runtime = "docker"
	Podman Runtime = "podman"
)

// ImageName is the default sandbox image tag.
const ImageName = "claude-mux-sandbox:latest"

// Mount describes a bind mount for a container.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerConfig holds all settings needed to run a sandbox container.
type ContainerConfig struct {
	Image        string
	Name         string
	Mounts       []Mount
	EnvVars      map[string]string
	Caps         []string // Linux capabilities (e.g. NET_ADMIN)
	SecurityOpts []string // --security-opt flags (e.g. no-new-privileges:true)
	Command      string   // shell command to run inside the container
	WorkDir      string
	User         string // uid:gid
	Remove       bool   // --rm
	Interactive  bool   // -it (allocate tty, needed for Claude Code)
}

// DetectRuntime checks for docker or podman in PATH.
func DetectRuntime() (Runtime, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		return Docker, nil
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return Podman, nil
	}
	return "", fmt.Errorf("neither docker nor podman found in PATH")
}
