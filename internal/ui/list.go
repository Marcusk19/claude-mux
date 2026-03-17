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
	session  session.ClaudeSession
	maxWidth int // available width for text wrapping
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

	left := fmt.Sprintf("%s %s", stateEmoji, stateStyle(i.session.State).Render(path))
	if i.session.GitBranch != "" {
		left += "  " + branchStyle.Render(i.session.GitBranch)
	}

	if i.session.LastActivity.IsZero() {
		return left
	}

	ago := agoStyle.Render("[" + timeAgo(i.session.LastActivity) + "]")
	leftWidth := lipgloss.Width(left)
	agoWidth := lipgloss.Width(ago)
	gap := i.maxWidth - leftWidth - agoWidth
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + ago
}

func (i sessionItem) Description() string {
	s := i.session

	// Line 1: summary or initial prompt
	line1 := s.Summary
	if line1 == "" {
		line1 = s.InitialPrompt
	}
	if line1 == "" {
		line1 = "No summary"
	}

	// Line 2+: live status or last assistant message, word-wrapped
	var activity string
	if s.LiveStatus != "" {
		activity = s.LiveStatus
	} else if s.CurrentActivity != "" {
		activity = s.CurrentActivity
	}

	if activity == "" {
		return line1
	}

	wrapped := wordWrap(activity, i.maxWidth, 2)
	return line1 + "\n" + activityStyle.Render("💬 "+wrapped)
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

// wordWrap wraps text to fit within maxWidth, breaking at word boundaries.
// Returns at most maxLines lines (default 2).
func wordWrap(s string, maxWidth int, maxLines int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	if maxLines <= 0 {
		maxLines = 2
	}
	// Account for the 💬 prefix (~4 chars with emoji width)
	lineWidth := maxWidth - 4

	var lines []string
	remaining := s
	for i := 0; i < maxLines && remaining != ""; i++ {
		if len(remaining) <= lineWidth {
			lines = append(lines, remaining)
			remaining = ""
			break
		}
		breakAt := lineWidth
		for breakAt > 0 && remaining[breakAt] != ' ' {
			breakAt--
		}
		if breakAt == 0 {
			breakAt = lineWidth
		}
		lines = append(lines, remaining[:breakAt])
		remaining = strings.TrimSpace(remaining[breakAt:])
	}
	// If there's still text left, truncate the last line
	if remaining != "" && len(lines) > 0 {
		last := lines[len(lines)-1]
		if len(last)+3 < lineWidth {
			lines[len(lines)-1] = last + "…"
		} else {
			lines[len(lines)-1] = last[:lineWidth-1] + "…"
		}
	}
	return strings.Join(lines, "\n   ")
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
