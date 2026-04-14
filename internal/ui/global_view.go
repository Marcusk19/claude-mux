package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/Marcusk19/claude-mux/internal/session"
	"github.com/Marcusk19/claude-mux/internal/windowname"
)

// groupHeaderItem represents a group header in the session list.
type groupHeaderItem struct {
	key       string
	name      string
	count     int
	collapsed bool
}

func (g groupHeaderItem) Title() string {
	indicator := "▼"
	detail := fmt.Sprintf("%d sessions", g.count)
	if g.collapsed {
		indicator = "▶"
		detail = "collapsed"
	}
	return fmt.Sprintf("%s %s (%s) ────", indicator, g.name, detail)
}

func (g groupHeaderItem) Description() string {
	return ""
}

func (g groupHeaderItem) FilterValue() string {
	return g.name
}

// selectableItem is an entry in the flat cursor list for grouped mode.
type selectableItem struct {
	isHeader bool
	groupKey string
	// Set when isHeader == true
	header groupHeaderItem
	// Set when isHeader == false
	session session.ClaudeSession
}

// sessionGroup holds sessions belonging to one tmux session:window.
type sessionGroup struct {
	key          string
	displayName  string
	sessions     []session.ClaudeSession
	lastActivity int64
}

// groupedSessionItems groups sessions by window and inserts group headers (for flat list / filter mode).
func groupedSessionItems(sessions []session.ClaudeSession, customNames map[string]string, collapsed map[string]bool, maxWidth int) []list.Item {
	if len(sessions) == 0 {
		return nil
	}

	groups := buildGroups(sessions, customNames)

	var items []list.Item
	for _, g := range groups {
		isCollapsed := collapsed[g.key]
		items = append(items, groupHeaderItem{
			key:       g.key,
			name:      g.displayName,
			count:     len(g.sessions),
			collapsed: isCollapsed,
		})
		if !isCollapsed {
			for _, s := range g.sessions {
				items = append(items, sessionItem{session: s, maxWidth: maxWidth})
			}
		}
	}

	return items
}

// buildGroups creates sorted session groups from a list of sessions.
func buildGroups(sessions []session.ClaudeSession, customNames map[string]string) []*sessionGroup {
	groupMap := make(map[string]*sessionGroup)
	for _, s := range sessions {
		key := windowname.GroupKey(s.Pane.SessionName, s.Pane.WindowIndex)
		g, ok := groupMap[key]
		if !ok {
			g = &sessionGroup{key: key}
			groupMap[key] = g
		}
		g.sessions = append(g.sessions, s)
		ts := s.LastActivity.Unix()
		if ts > g.lastActivity {
			g.lastActivity = ts
		}
	}

	for key, g := range groupMap {
		var paths []string
		for _, s := range g.sessions {
			paths = append(paths, s.Pane.PanePath)
		}
		g.displayName = windowname.DisplayName(key, customNames, paths)
	}

	groups := make([]*sessionGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].lastActivity > groups[j].lastActivity
	})

	return groups
}

// buildSelectableItems creates a flat list of selectable items for cursor navigation.
func buildSelectableItems(sessions []session.ClaudeSession, customNames map[string]string, collapsed map[string]bool) []selectableItem {
	groups := buildGroups(sessions, customNames)

	var items []selectableItem
	for _, g := range groups {
		isCollapsed := collapsed[g.key]
		items = append(items, selectableItem{
			isHeader: true,
			groupKey: g.key,
			header: groupHeaderItem{
				key:       g.key,
				name:      g.displayName,
				count:     len(g.sessions),
				collapsed: isCollapsed,
			},
		})
		if !isCollapsed {
			for _, s := range g.sessions {
				items = append(items, selectableItem{
					isHeader: false,
					groupKey: g.key,
					session:  s,
				})
			}
		}
	}

	return items
}

var (
	groupBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	groupBoxSelectedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(0, 1)

	groupTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Bold(true)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("117"))

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
)

// renderGroupedGlobal renders the grouped view with boxes around each group.
func renderGroupedGlobal(
	sessions []session.ClaudeSession,
	customNames map[string]string,
	collapsed map[string]bool,
	cursor int,
	scroll int,
	width int,
	height int,
	markedPanes map[string]bool,
) string {
	if len(sessions) == 0 {
		return "\n  No sessions found"
	}

	if height <= 0 || width <= 0 {
		return ""
	}

	items := buildSelectableItems(sessions, customNames, collapsed)
	if cursor >= len(items) {
		cursor = len(items) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	groups := buildGroups(sessions, customNames)
	boxWidth := width - 4 // account for outer padding
	if boxWidth < 30 {
		boxWidth = 30
	}
	contentWidth := boxWidth - 4 // account for box border + padding

	// Build rendered groups
	var renderedGroups []string
	itemIdx := 0
	for _, g := range groups {
		isCollapsed := collapsed[g.key]

		// Determine if any item in this group is selected
		groupHasSelection := false
		groupStartIdx := itemIdx

		// Header
		headerSelected := cursor == itemIdx
		if headerSelected {
			groupHasSelection = true
		}
		itemIdx++

		// Sessions
		sessionCount := 0
		if !isCollapsed {
			for range g.sessions {
				if cursor == itemIdx {
					groupHasSelection = true
				}
				itemIdx++
				sessionCount++
			}
		}

		// Render header line
		indicator := "▼"
		if isCollapsed {
			indicator = "▶"
		}
		countStr := fmt.Sprintf("%d sessions", len(g.sessions))

		var headerLine string
		if headerSelected {
			headerLine = fmt.Sprintf("%s %s (%s)",
				indicator,
				selectedItemStyle.Bold(true).Render(g.displayName),
				dimStyle.Render(countStr))
		} else {
			headerLine = fmt.Sprintf("%s %s (%s)",
				indicator,
				groupTitleStyle.Render(g.displayName),
				dimStyle.Render(countStr))
		}

		var lines []string
		lines = append(lines, headerLine)

		// Render session items within the group
		if !isCollapsed {
			for i, s := range g.sessions {
				sessionIdx := groupStartIdx + 1 + i
				isSelected := cursor == sessionIdx
				marked := markedPanes[s.Pane.PaneID]
				lines = append(lines, renderGroupSession(s, isSelected, contentWidth, marked))
			}
		}

		content := strings.Join(lines, "\n")

		// Use highlighted box if group contains selection
		style := groupBoxStyle.Width(contentWidth)
		if groupHasSelection {
			style = groupBoxSelectedStyle.Width(contentWidth)
		}

		renderedGroups = append(renderedGroups, style.Render(content))
	}

	// Join all groups with a small gap
	fullView := strings.Join(renderedGroups, "\n")

	// Handle scrolling
	viewLines := strings.Split(fullView, "\n")
	if len(viewLines) <= height {
		return fullView
	}

	// Adjust scroll to keep cursor visible
	// Find which lines correspond to the cursor
	cursorLineStart, cursorLineEnd := findCursorLines(renderedGroups, groups, collapsed, cursor)
	if cursorLineEnd > scroll+height {
		scroll = cursorLineEnd - height + 1
	}
	if cursorLineStart < scroll {
		scroll = cursorLineStart
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll > len(viewLines)-height {
		scroll = len(viewLines) - height
	}

	visible := viewLines[scroll:]
	if len(visible) > height {
		visible = visible[:height]
	}
	return strings.Join(visible, "\n")
}

// renderGroupSession renders a single session within a group box.
func renderGroupSession(s session.ClaudeSession, selected bool, maxWidth int, marked bool) string {
	stateEmoji := s.State.Emoji()
	if marked {
		stateEmoji = "☑ " + stateEmoji
	}
	if s.Pinned {
		stateEmoji = "📌" + stateEmoji
	}

	path := shortenPath(s.ProjectPath)
	nameStyle := normalItemStyle
	if selected {
		nameStyle = selectedItemStyle
	}

	// Line 1: state + path + branch + time ago
	line1 := fmt.Sprintf("  %s %s", stateEmoji, nameStyle.Render(path))
	if s.GitBranch != "" {
		line1 += "  " + branchStyle.Render(s.GitBranch)
	}
	if !s.LastActivity.IsZero() {
		ago := agoStyle.Render("[" + timeAgo(s.LastActivity) + "]")
		leftWidth := lipgloss.Width(line1)
		agoWidth := lipgloss.Width(ago)
		gap := maxWidth - leftWidth - agoWidth
		if gap < 2 {
			gap = 2
		}
		line1 += strings.Repeat(" ", gap) + ago
	}

	// Line 2: summary
	summary := s.Summary
	if summary == "" {
		summary = s.InitialPrompt
	}
	if summary == "" {
		summary = "No summary"
	}
	line2 := "    " + dimStyle.Render(truncate(summary, maxWidth-4))

	// Line 3: activity (if any)
	activity := s.LiveStatus
	if activity == "" {
		activity = s.CurrentActivity
	}
	if activity != "" {
		line3 := "    " + activityStyle.Render("💬 "+truncate(activity, maxWidth-8))
		return line1 + "\n" + line2 + "\n" + line3
	}

	return line1 + "\n" + line2
}

// truncate shortens a string to maxLen, adding ellipsis if needed.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// findCursorLines returns the start and end line indices for the item at the given cursor position.
func findCursorLines(renderedGroups []string, groups []*sessionGroup, collapsed map[string]bool, cursor int) (int, int) {
	lineOffset := 0
	itemIdx := 0

	for gi, g := range groups {
		groupLines := strings.Count(renderedGroups[gi], "\n") + 1

		// Header
		if cursor == itemIdx {
			return lineOffset, lineOffset + groupLines - 1
		}
		itemIdx++

		if !collapsed[g.key] {
			// Approximate: header is ~1 line, each session is ~2-3 lines, within the box
			sessionLineStart := lineOffset + 2 // after box top border + header
			linesPerSession := 3
			for i := range g.sessions {
				if cursor == itemIdx {
					start := sessionLineStart + i*linesPerSession
					return start, start + linesPerSession - 1
				}
				itemIdx++
			}
		}

		lineOffset += groupLines + 1 // +1 for gap between groups
	}

	return 0, 0
}
