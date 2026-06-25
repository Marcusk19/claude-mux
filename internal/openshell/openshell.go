package openshell

import (
	"fmt"
	"os/exec"
	"strings"
)

// claudeEnvPrefix is the environment variable prefix needed for Claude Code
// to use the openshell inference gateway. DISABLE_AUTOUPDATER prevents the
// auto-update check from failing inside the sandbox (filesystem is read-only).
const claudeEnvPrefix = "DISABLE_AUTOUPDATER=1 CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1 ANTHROPIC_BASE_URL=https://inference.local ANTHROPIC_API_KEY=unused"

// Available reports whether the openshell CLI is in PATH.
func Available() bool {
	_, err := exec.LookPath("openshell")
	return err == nil
}

// BuildInteractiveCommand builds a shell command for an interactive sandbox-split session.
// The sandbox persists after exit (no --no-keep) so the user can reconnect
// with `openshell sandbox connect <name>`.
func BuildInteractiveCommand(name string, workDir string, provider string) string {
	args := []string{
		"openshell", "sandbox", "create",
		"--name", name,
		"--upload", workDir,
	}
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	args = append(args, "--",
		"sh", "-c",
		fmt.Sprintf("%s claude --bare", claudeEnvPrefix),
	)
	return shellJoin(args)
}

// BuildAutonomousCommand builds a shell command for an autonomous subagent sandbox.
// Uses --no-keep for auto-cleanup after claude exits.
func BuildAutonomousCommand(name string, workDir string, provider string) string {
	args := []string{
		"openshell", "sandbox", "create",
		"--name", name,
		"--no-keep",
		"--upload", workDir,
	}
	if provider != "" {
		args = append(args, "--provider", provider)
	}

	claudeCmd := fmt.Sprintf(
		`%s claude -p --bare --dangerously-skip-permissions --append-system-prompt "$(cat .claude-mux/system-prompt.txt)" "$(cat .claude-mux/prompt.txt)"`,
		claudeEnvPrefix,
	)
	args = append(args, "--", "sh", "-c", claudeCmd)
	return shellJoin(args)
}

// shellJoin produces a single shell-safe command string from args.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps a string in single quotes if it contains special characters.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
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
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func isShellSafe(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '=' || c == '+'
}
