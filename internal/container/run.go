package container

import (
	"fmt"
	"strings"
)

// BuildRunArgs produces the argument list for "docker run" / "podman run".
func BuildRunArgs(cfg ContainerConfig) []string {
	args := []string{"run"}

	if cfg.Interactive {
		args = append(args, "-it")
	}

	if cfg.Remove {
		args = append(args, "--rm")
	}

	if cfg.Name != "" {
		args = append(args, "--name", cfg.Name)
	}

	if cfg.User != "" {
		args = append(args, "--user", cfg.User)
	}

	if cfg.WorkDir != "" {
		args = append(args, "-w", cfg.WorkDir)
	}

	for _, cap := range cfg.Caps {
		args = append(args, "--cap-add="+cap)
	}

	for _, opt := range cfg.SecurityOpts {
		args = append(args, "--security-opt="+opt)
	}

	for _, m := range cfg.Mounts {
		spec := fmt.Sprintf("%s:%s", m.Source, m.Target)
		if m.ReadOnly {
			spec += ":ro"
		}
		args = append(args, "-v", spec)
	}

	for k, v := range cfg.EnvVars {
		args = append(args, "-e", k+"="+v)
	}

	args = append(args, cfg.Image)

	if cfg.Command != "" {
		args = append(args, "sh", "-c", cfg.Command)
	}

	return args
}

// BuildShellCommand returns a single shell-escaped command string suitable
// for tmux split-window. Format: "docker run ... <image> sh -c '...'"
func BuildShellCommand(runtime Runtime, cfg ContainerConfig) string {
	args := BuildRunArgs(cfg)
	// Prepend the runtime binary
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, string(runtime))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps a string in single quotes if it contains special characters.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// If the string is clean, return as-is
	clean := true
	for _, c := range s {
		if !isShellSafe(c) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	// Single-quote the string, escaping any embedded single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func isShellSafe(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '=' || c == '+'
}
