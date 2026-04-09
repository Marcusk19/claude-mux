package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Marcusk19/claude-mux/devcontainer"
	"github.com/Marcusk19/claude-mux/internal/container"
)

// SandboxSplitOpts configures a sandboxed split pane.
type SandboxSplitOpts struct {
	SplitFlag string // "-h" (vertical/side-by-side) or "-v" (horizontal/stacked)
	PanePath  string // working directory of the current pane
}

// SandboxSplit builds the sandbox image if needed, and opens a tmux split with
// an interactive Claude session inside a sandboxed container. The current
// directory is bind-mounted directly — no worktree is created.
func SandboxSplit(opts SandboxSplitOpts) error {
	// Build sandbox image
	runtime, err := container.DetectRuntime()
	if err != nil {
		return tmuxMessage("sandbox requires docker or podman")
	}
	if err := container.EnsureImage(runtime, devcontainer.Assets); err != nil {
		return tmuxMessage(fmt.Sprintf("sandbox image build failed: %v", err))
	}
	if !container.ImageExists(runtime) {
		return tmuxMessage("sandbox image not found after build")
	}

	absPath, err := filepath.Abs(opts.PanePath)
	if err != nil {
		absPath = opts.PanePath
	}

	taskDir := filepath.Join(absPath, ".claude-mux")
	os.MkdirAll(taskDir, 0o755)

	// Stage config copies outside the workspace so they don't overlap with
	// the /workspace bind mount. Uses ~/.cache/claude-mux/sandbox-config/.
	home, _ := os.UserHomeDir()
	stageDir := filepath.Join(home, ".cache", "claude-mux", "sandbox-config")
	os.RemoveAll(stageDir)
	os.MkdirAll(stageDir, 0o755)
	stageClaudeConfig(stageDir, home)

	entryScript := `#!/bin/sh
set -e
/usr/local/bin/init-firewall.sh
chown -R node:node /workspace 2>/dev/null || true
chown -R node:node /home/node/.claude /home/node/.claude.json 2>/dev/null || true
chown -R node:node /home/node/.config 2>/dev/null || true
export HOME=/home/node
su -s /bin/sh node -c '/usr/local/bin/security-init.sh'
exec su -s /bin/sh node -c 'cd /workspace && exec claude --bare'
`
	entryScriptPath := filepath.Join(taskDir, "sandbox-entry.sh")
	if err := os.WriteFile(entryScriptPath, []byte(entryScript), 0o755); err != nil {
		return fmt.Errorf("writing entry script: %w", err)
	}

	// Build container command
	taskID := generateTaskID()
	containerName := "claude-mux-" + taskID
	dockerCmd := buildInteractiveSandboxCommand(runtime, absPath, containerName, stageDir)

	// Open tmux split
	splitArgs := []string{
		"split-window",
		opts.SplitFlag,
		"-c", absPath,
		dockerCmd,
	}

	if out, err := exec.Command("tmux", splitArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux split-window: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// buildInteractiveSandboxCommand constructs a docker/podman run command for an
// interactive (non-autonomous) sandboxed Claude session. Config files are
// staged into stageDir (copies, not originals) so chown inside the container
// doesn't mutate host files.
func buildInteractiveSandboxCommand(runtime container.Runtime, workDir string, containerName string, stageDir string) string {
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "claude-mux")

	cfg := container.ContainerConfig{
		Image:        container.ImageName,
		Name:         containerName,
		Remove:       true,
		Interactive:  true,
		WorkDir:      "/workspace",
		User:         "0:0",
		Caps:         []string{"NET_ADMIN", "NET_RAW"},
		SecurityOpts: []string{"no-new-privileges:true"},
		Command:      "/workspace/.claude-mux/sandbox-entry.sh",
		Mounts: []container.Mount{
			{Source: workDir, Target: "/workspace"},
			{Source: cacheDir, Target: cacheDir},
		},
		EnvVars: map[string]string{
			"CLAUDE_MUX_CACHE": cacheDir,
		},
	}

	// Pass auth credentials
	authEnvVars := []string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_CODE_USE_VERTEX",
		"ANTHROPIC_VERTEX_PROJECT_ID",
		"CLOUD_ML_REGION",
		"CLOUD_ML_PROJECT_ID",
		"GOOGLE_APPLICATION_CREDENTIALS",
	}
	for _, key := range authEnvVars {
		if val := os.Getenv(key); val != "" {
			cfg.EnvVars[key] = val
		}
	}

	// Mount staged copies of config (safe to chown inside the container).
	claudeDir := filepath.Join(stageDir, ".claude")
	if info, err := os.Stat(claudeDir); err == nil && info.IsDir() {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: claudeDir, Target: "/home/node/.claude",
		})
	}

	claudeJSON := filepath.Join(stageDir, ".claude.json")
	if _, err := os.Stat(claudeJSON); err == nil {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: claudeJSON, Target: "/home/node/.claude.json",
		})
	}

	gcloudDir := filepath.Join(stageDir, ".config", "gcloud")
	if info, err := os.Stat(gcloudDir); err == nil && info.IsDir() {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: gcloudDir, Target: "/home/node/.config/gcloud",
		})
	}

	// These are read-only, safe to mount from host directly.
	gitconfig := filepath.Join(home, ".gitconfig")
	if _, err := os.Stat(gitconfig); err == nil {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: gitconfig, Target: "/home/node/.gitconfig", ReadOnly: true,
		})
	}

	sshDir := filepath.Join(home, ".ssh")
	if _, err := os.Stat(sshDir); err == nil {
		cfg.Mounts = append(cfg.Mounts, container.Mount{
			Source: sshDir, Target: "/home/node/.ssh", ReadOnly: true,
		})
	}

	return container.BuildShellCommand(runtime, cfg)
}

// stageClaudeConfig copies Claude and gcloud config into a staging directory
// so the container can chown them without mutating host files.
func stageClaudeConfig(stageDir, home string) {
	// Copy ~/.claude directory
	claudeSrc := filepath.Join(home, ".claude")
	if info, err := os.Stat(claudeSrc); err == nil && info.IsDir() {
		claudeDst := filepath.Join(stageDir, ".claude")
		copyDir(claudeSrc, claudeDst)
	}

	// Copy ~/.claude.json
	claudeJSON := filepath.Join(home, ".claude.json")
	if _, err := os.Stat(claudeJSON); err == nil {
		copyFile(claudeJSON, filepath.Join(stageDir, ".claude.json"))
	}

	// Copy ~/.config/gcloud
	gcloudSrc := filepath.Join(home, ".config", "gcloud")
	if info, err := os.Stat(gcloudSrc); err == nil && info.IsDir() {
		gcloudDst := filepath.Join(stageDir, ".config", "gcloud")
		os.MkdirAll(filepath.Dir(gcloudDst), 0o755)
		copyDir(gcloudSrc, gcloudDst)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Skip large files (session transcripts, etc.) — only need config
		if info.Size() > 10*1024*1024 {
			return nil
		}
		return copyFile(path, target)
	})
}

func tmuxMessage(msg string) error {
	return exec.Command("tmux", "display-message", "claude-mux: "+msg).Run()
}
