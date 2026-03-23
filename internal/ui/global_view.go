package ui

import (
	"sort"

	"github.com/charmbracelet/bubbles/list"
	"github.com/mkok/claude-mux/internal/session"
	"github.com/mkok/claude-mux/internal/windowname"
)

// groupHeaderItem represents a group header in the session list.
type groupHeaderItem struct {
	key       string
	name      string
	count     int
	collapsed bool
}

func (g groupHeaderItem) Title() string {
	return windowname.FormatGroupHeader(g.name, g.count, g.collapsed)
}

func (g groupHeaderItem) Description() string {
	return ""
}

func (g groupHeaderItem) FilterValue() string {
	return g.name
}

// sessionGroup holds sessions belonging to one tmux session:window.
type sessionGroup struct {
	key          string
	displayName  string
	sessions     []session.ClaudeSession
	lastActivity int64 // unix timestamp of most recent session
}

// groupedSessionItems groups sessions by window and inserts group headers.
func groupedSessionItems(sessions []session.ClaudeSession, customNames map[string]string, collapsed map[string]bool, maxWidth int) []list.Item {
	if len(sessions) == 0 {
		return nil
	}

	// Group sessions by tmux session:window
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

	// Compute display names
	for key, g := range groupMap {
		var paths []string
		for _, s := range g.sessions {
			paths = append(paths, s.Pane.PanePath)
		}
		g.displayName = windowname.DisplayName(key, customNames, paths)
	}

	// Sort groups by most recent activity
	groups := make([]*sessionGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].lastActivity > groups[j].lastActivity
	})

	// Build flat list with headers
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
