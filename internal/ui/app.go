package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mkok/claude-mux/internal/kanban"
	"github.com/mkok/claude-mux/internal/pin"
	"github.com/mkok/claude-mux/internal/session"
	"github.com/mkok/claude-mux/internal/tmux"
	"github.com/mkok/claude-mux/internal/windowname"
	"github.com/mkok/claude-mux/internal/worktree"
)

const pollInterval = 2 * time.Second

// sessionsMsg carries discovered sessions to the TUI.
type sessionsMsg []session.ClaudeSession

// worktreesMsg carries discovered worktrees to the TUI.
type worktreesMsg []worktree.Worktree

// removeResultMsg carries the result of a worktree removal.
type removeResultMsg struct {
	path  string
	err   error
	force bool // whether this was already a --force attempt
}

// tickMsg triggers a poll for sessions.
type tickMsg time.Time

// Model is the Bubble Tea model for the session list.
type Model struct {
	list          list.Model
	worktreeList  list.Model
	activeTab     Tab
	selected      *session.ClaudeSession
	selectedPane  *tmux.PaneInfo
	quitting      bool
	width         int
	height        int
	totalCount    int
	worktreeCount int
	confirmRemove *worktree.Worktree
	forceRemove   bool   // true when confirming a force removal after normal remove failed
	statusMessage string
	sessions        []session.ClaudeSession // keep for cross-referencing
	kanbanCards     []kanban.PaneCard
	kanbanSession   string // tmux session name for current window
	kanbanWindow    string // tmux window index for current window
	selectedCard    int    // selected card index in kanban view
	globalGrouped   bool
	collapsedGroups map[string]bool
	windowNames     map[string]string
	renaming        bool
	renameInput     string
	renameTarget    string
}

// Selected returns the session the user chose, or nil if they quit.
func (m *Model) Selected() *session.ClaudeSession {
	return m.selected
}

// SelectedPane returns a pane to jump to (from worktree tab), or nil.
func (m *Model) SelectedPane() *tmux.PaneInfo {
	return m.selectedPane
}

// NewModel creates a new TUI model.
func NewModel(kanbanSession, kanbanWindow string) *Model {
	// Session list delegate
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(4)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("117")).
		BorderForeground(lipgloss.Color("62"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("245")).
		BorderForeground(lipgloss.Color("62"))

	l := list.New(nil, delegate, 0, 0)
	l.Title = "Claude Code Sessions"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.DisableQuitKeybindings()
	l.SetStatusBarItemName("session", "sessions")

	// Worktree list delegate
	wtDelegate := list.NewDefaultDelegate()
	wtDelegate.SetHeight(2)
	wtDelegate.Styles.SelectedTitle = wtDelegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("117")).
		BorderForeground(lipgloss.Color("62"))
	wtDelegate.Styles.SelectedDesc = wtDelegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("245")).
		BorderForeground(lipgloss.Color("62"))

	wl := list.New(nil, wtDelegate, 0, 0)
	wl.Title = "Git Worktrees"
	wl.Styles.Title = titleStyle
	wl.SetShowStatusBar(false)
	wl.SetFilteringEnabled(true)
	wl.SetShowHelp(true)
	wl.DisableQuitKeybindings()
	wl.SetStatusBarItemName("worktree", "worktrees")

	return &Model{
		list:            l,
		worktreeList:    wl,
		kanbanSession:   kanbanSession,
		kanbanWindow:    kanbanWindow,
		globalGrouped:   true,
		collapsedGroups: make(map[string]bool),
		windowNames:     windowname.Load(),
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(pollSessions, pollWorktrees, m.pollKanbanCmd(), tickCmd())
}

func (m *Model) pollKanbanCmd() tea.Cmd {
	return pollKanbanCmd(m.kanbanSession, m.kanbanWindow)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h, v := appStyle.GetFrameSize()
		listHeight := msg.Height - v - tabBarHeight()
		m.list.SetSize(msg.Width-h, listHeight)
		m.worktreeList.SetSize(msg.Width-h, listHeight)

	case sessionsMsg:
		m.sessions = []session.ClaudeSession(msg)
		m.totalCount = len(msg)
		cmd := m.list.SetItems(m.buildGlobalItems())
		return m, cmd

	case worktreesMsg:
		items := make([]list.Item, len(msg))
		for i, wt := range msg {
			items[i] = worktreeItem{wt: wt}
		}
		m.worktreeCount = len(msg)
		cmd := m.worktreeList.SetItems(items)
		return m, cmd

	case kanbanMsg:
		m.kanbanCards = []kanban.PaneCard(msg)
		if m.selectedCard >= len(m.kanbanCards) && len(m.kanbanCards) > 0 {
			m.selectedCard = len(m.kanbanCards) - 1
		}
		return m, nil

	case removeResultMsg:
		if msg.err != nil {
			if msg.force {
				// Force removal also failed — nothing more we can do
				m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
				m.confirmRemove = nil
				m.forceRemove = false
			} else {
				// Normal remove failed — offer force removal
				m.statusMessage = fmt.Sprintf("%v", msg.err)
				m.forceRemove = true
				// Re-find the worktree to populate confirmRemove for the force prompt
				if m.confirmRemove == nil {
					for i := range m.worktreeList.Items() {
						if item, ok := m.worktreeList.Items()[i].(worktreeItem); ok && item.wt.Path == msg.path {
							m.confirmRemove = &item.wt
							break
						}
					}
				}
			}
		} else {
			m.statusMessage = fmt.Sprintf("Removed %s", shortenPath(msg.path))
			m.confirmRemove = nil
			m.forceRemove = false
		}
		return m, pollWorktrees

	case tickMsg:
		return m, tea.Batch(pollSessions, m.pollKanbanCmd(), tickCmd())

	case tea.KeyMsg:
		// Handle confirmation prompt first
		if m.confirmRemove != nil {
			switch msg.String() {
			case "y", "Y":
				wt := m.confirmRemove
				force := m.forceRemove
				if !force {
					// Keep confirmRemove set so we can re-populate it on failure
				} else {
					m.confirmRemove = nil
				}
				m.forceRemove = false
				return m, removeWorktreeCmd(wt.RepoRoot, wt.Path, force)
			case "n", "N", "esc":
				m.confirmRemove = nil
				m.forceRemove = false
				m.statusMessage = ""
				return m, nil
			}
			return m, nil
		}

		// Handle rename mode
		if m.renaming {
			switch msg.Type {
			case tea.KeyEnter:
				m.windowNames[m.renameTarget] = m.renameInput
				_ = windowname.Save(m.windowNames)
				m.renaming = false
				cmd := m.list.SetItems(m.buildGlobalItems())
				return m, cmd
			case tea.KeyEsc:
				m.renaming = false
				return m, nil
			case tea.KeyBackspace:
				if len(m.renameInput) > 0 {
					m.renameInput = m.renameInput[:len(m.renameInput)-1]
				}
				return m, nil
			case tea.KeyRunes:
				m.renameInput += string(msg.Runes)
				return m, nil
			}
			return m, nil
		}

		// Don't intercept keys while filtering
		activeList := m.activeList()
		if activeList.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "left", "h":
			if m.activeTab == TabKanban && m.selectedCard > 0 {
				m.selectedCard--
				return m, nil
			}
		case "right", "l":
			if m.activeTab == TabKanban && m.selectedCard < len(m.kanbanCards)-1 {
				m.selectedCard++
				return m, nil
			}
		case "tab":
			m.statusMessage = ""
			m.activeTab = Tab((int(m.activeTab) + 1) % len(tabNames))
			if m.activeTab == TabWorktrees {
				return m, pollWorktrees
			}
			return m, nil
		case "shift+tab":
			m.statusMessage = ""
			m.activeTab = Tab((int(m.activeTab) + len(tabNames) - 1) % len(tabNames))
			if m.activeTab == TabWorktrees {
				return m, pollWorktrees
			}
			return m, nil
		case "enter":
			if m.activeTab == TabGlobal {
				if _, ok := m.list.SelectedItem().(groupHeaderItem); ok {
					return m, m.handleGroupHeaderEnter()
				}
			}
			return m, m.handleEnter()
		case "g":
			if m.activeTab == TabGlobal {
				m.globalGrouped = !m.globalGrouped
				cmd := m.list.SetItems(m.buildGlobalItems())
				return m, cmd
			}
		case "r":
			if m.activeTab == TabGlobal {
				if item, ok := m.list.SelectedItem().(groupHeaderItem); ok {
					m.renaming = true
					m.renameTarget = item.key
					m.renameInput = item.name
					return m, nil
				}
			}
		case "p":
			if m.activeTab == TabGlobal {
				if item, ok := m.list.SelectedItem().(sessionItem); ok {
					pin.Toggle(item.session.ProjectPath)
					return m, pollSessions
				}
			}
		case "d", "x":
			if m.activeTab == TabWorktrees {
				return m, m.handleRemove()
			}
		}
	}

	// Delegate to active list (skip for kanban which doesn't use a list)
	if m.activeTab == TabKanban {
		return m, nil
	}
	var cmd tea.Cmd
	if m.activeTab == TabGlobal {
		m.list, cmd = m.list.Update(msg)
	} else {
		m.worktreeList, cmd = m.worktreeList.Update(msg)
	}
	return m, cmd
}

func (m *Model) activeList() *list.Model {
	switch m.activeTab {
	case TabWorktrees:
		return &m.worktreeList
	case TabKanban:
		return &m.list // kanban doesn't use a list, but return something to avoid nil
	default:
		return &m.list
	}
}

func (m *Model) handleEnter() tea.Cmd {
	if m.activeTab == TabKanban {
		if m.selectedCard < len(m.kanbanCards) {
			pane := m.kanbanCards[m.selectedCard].Pane
			m.selectedPane = &pane
			m.quitting = true
			return tea.Quit
		}
		return nil
	}
	if m.activeTab == TabGlobal {
		if item, ok := m.list.SelectedItem().(sessionItem); ok {
			s := item.session
			m.selected = &s
			m.quitting = true
			return tea.Quit
		}
	} else {
		if item, ok := m.worktreeList.SelectedItem().(worktreeItem); ok {
			if item.wt.HasSession && item.wt.PaneID != "" {
				pane := m.findPaneByID(item.wt.PaneID)
				if pane != nil {
					m.selectedPane = pane
					m.quitting = true
					return tea.Quit
				}
			}
		}
	}
	return nil
}

func (m *Model) handleRemove() tea.Cmd {
	item, ok := m.worktreeList.SelectedItem().(worktreeItem)
	if !ok {
		return nil
	}
	if item.wt.IsMain {
		m.statusMessage = "Cannot remove the main worktree"
		return nil
	}
	m.confirmRemove = &item.wt
	if item.wt.HasSession {
		m.statusMessage = "Warning: Active Claude session!"
	} else {
		m.statusMessage = ""
	}
	return nil
}

func (m *Model) buildGlobalItems() []list.Item {
	itemWidth := m.width - 8
	if itemWidth < 40 {
		itemWidth = 40
	}
	if m.globalGrouped {
		return groupedSessionItems(m.sessions, m.windowNames, m.collapsedGroups, itemWidth)
	}
	items := make([]list.Item, len(m.sessions))
	for i, s := range m.sessions {
		items[i] = sessionItem{session: s, maxWidth: itemWidth}
	}
	return items
}

func (m *Model) handleGroupHeaderEnter() tea.Cmd {
	if item, ok := m.list.SelectedItem().(groupHeaderItem); ok {
		m.collapsedGroups[item.key] = !m.collapsedGroups[item.key]
		return m.list.SetItems(m.buildGlobalItems())
	}
	return nil
}

func (m *Model) findPaneByID(paneID string) *tmux.PaneInfo {
	for _, s := range m.sessions {
		if s.Pane.PaneID == paneID {
			p := s.Pane
			return &p
		}
	}
	return nil
}

func (m *Model) View() string {
	if m.quitting {
		return ""
	}

	tabs := renderTabBar(m.activeTab, m.width-4) // account for appStyle padding

	var content string
	switch m.activeTab {
	case TabKanban:
		h, v := appStyle.GetFrameSize()
		kanbanHeight := m.height - v - tabBarHeight()
		content = renderKanban(m.kanbanCards, m.selectedCard, m.width-h, kanbanHeight)
	case TabGlobal:
		var footer string
		if m.renaming {
			footer = confirmStyle.Render(fmt.Sprintf("Rename: %s█", m.renameInput))
		} else {
			footer = footerStyle.Render(fmt.Sprintf(" %d/%d sessions ", m.list.Index()+1, m.totalCount))
		}
		content = m.list.View() + "\n" + footer
	default: // TabWorktrees
		var footer string
		if m.confirmRemove != nil {
			var prompt string
			if m.forceRemove {
				prompt = fmt.Sprintf("%s — Force remove? (y/n)", m.statusMessage)
			} else {
				prompt = fmt.Sprintf("Remove %s? ", shortenPath(m.confirmRemove.Path))
				if m.statusMessage != "" {
					prompt = m.statusMessage + " " + prompt
				}
				prompt += "(y/n)"
			}
			footer = confirmStyle.Render(prompt)
		} else if m.statusMessage != "" {
			footer = statusStyle.Render(m.statusMessage)
		} else {
			footer = footerStyle.Render(fmt.Sprintf(" %d/%d worktrees ", m.worktreeList.Index()+1, m.worktreeCount))
		}
		content = m.worktreeList.View() + "\n" + footer
	}

	return appStyle.Render(tabs + content)
}

func pollSessions() tea.Msg {
	sessions, err := session.DiscoverSessions()
	if err != nil {
		return sessionsMsg(nil)
	}
	return sessionsMsg(sessions)
}

func pollWorktrees() tea.Msg {
	panes, err := tmux.ListPanes()
	if err != nil {
		return worktreesMsg(nil)
	}

	// Collect unique pane paths and build a set of Claude pane paths+IDs
	pathSet := make(map[string]bool)
	type claudePane struct {
		path   string
		paneID string
	}
	var claudePanes []claudePane
	for _, p := range panes {
		pathSet[p.PanePath] = true
		if tmux.IsClaudePane(p) {
			claudePanes = append(claudePanes, claudePane{path: p.PanePath, paneID: p.PaneID})
		}
	}

	var paths []string
	for p := range pathSet {
		paths = append(paths, p)
	}

	wts, err := worktree.DiscoverWorktrees(paths)
	if err != nil {
		return worktreesMsg(nil)
	}

	// Mark worktrees that have active Claude sessions
	for i := range wts {
		for _, cp := range claudePanes {
			if cp.path == wts[i].Path {
				wts[i].HasSession = true
				wts[i].PaneID = cp.paneID
				break
			}
		}
	}

	return worktreesMsg(wts)
}

func removeWorktreeCmd(repoRoot, path string, force bool) tea.Cmd {
	return func() tea.Msg {
		err := worktree.Remove(repoRoot, path, force)
		return removeResultMsg{path: path, err: err, force: force}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
