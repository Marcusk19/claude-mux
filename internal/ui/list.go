package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mkok/claude-mux/internal/session"
)

// sessionItem implements the list.DefaultItem interface for bubbles list.
type sessionItem struct {
	session session.ClaudeSession
}

func stateStyle(s session.ActivityState) lipgloss.Style {
	switch s {
	case session.StateWorking:
		return workingStyle.Bold(true)
	case session.StateWaiting:
		return waitingStyle.Bold(true)
	case session.StatePermission:
		return permissionStyle.Bold(true)
	case session.StateDone:
		return doneStyle.Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	}
}

func (i sessionItem) Title() string {
	path := shortenPath(i.session.ProjectPath)
	stateEmoji := i.session.State.Emoji()
	if i.session.Pinned {
		stateEmoji = "📌" + stateEmoji
	}

	title := fmt.Sprintf("%s %s", stateEmoji, stateStyle(i.session.State).Render(path))
	if i.session.GitBranch != "" {
		title += "  " + branchStyle.Render(i.session.GitBranch)
	}
	return title
}

func (i sessionItem) Description() string {
	desc := i.session.Summary
	if desc == "" {
		desc = i.session.InitialPrompt
	}
	if desc == "" {
		desc = "No summary"
	}

	if !i.session.LastActivity.IsZero() {
		desc += "  " + agoStyle.Render("["+timeAgo(i.session.LastActivity)+"]")
	}
	return desc
}

func (i sessionItem) FilterValue() string {
	return i.session.ProjectPath + " " + i.session.Summary + " " + i.session.GitBranch
}

// shortenPath replaces the home directory with ~ and shortens the path.
func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	return p
}

// timeAgo returns a human-readable "time ago" string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
