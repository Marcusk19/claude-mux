package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Marcusk19/claude-mux/internal/cc"
	"github.com/Marcusk19/claude-mux/internal/kanban"
	"github.com/Marcusk19/claude-mux/internal/pin"
	"github.com/Marcusk19/claude-mux/internal/session"
	"github.com/Marcusk19/claude-mux/internal/tmux"
	"github.com/Marcusk19/claude-mux/internal/windowname"
	"github.com/Marcusk19/claude-mux/internal/worktree"
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

// ccStatusMsg carries the CC state to the TUI.
type ccStatusMsg struct{ state *cc.State }

// ccExecDoneMsg is sent when the CC tea.Exec process finishes.
type ccExecDoneMsg struct{ err error }

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
	kanbanBoard     *kanban.Board
	kanbanViewBoard KanbanBoard // computed view-layer board
	kanbanSession   string      // tmux session name for current window
	kanbanWindow    string      // tmux window index for current window
	selectedCol     int         // selected column (0-2) in kanban view
	selectedRow     int         // selected row within column
	globalGrouped   bool
	collapsedGroups map[string]bool
	windowNames     map[string]string
	renaming        bool
	renameInput     string
	renameTarget    string
	globalCursor    int
	globalScroll    int
	globalItems     []selectableItem // cached selectable items for grouped mode
	ccState         *cc.State
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
	return tea.Batch(pollSessions, pollWorktrees, m.pollKanbanCmd(), pollCCStatus, tickCmd())
}

func (m *Model) pollKanbanCmd() tea.Cmd {
	sessionName := m.kanbanSession
	windowIndex := m.kanbanWindow
	return func() tea.Msg {
		cards, _ := kanban.DiscoverKanban(sessionName, windowIndex)
		return kanbanMsg(cards)
	}
}

func pollKanbanBoardCmd(kanbanCards []kanban.PaneCard) tea.Cmd {
	return func() tea.Msg {
		// Derive repo root from the first pane card's path
		var repoRoot string
		for _, pc := range kanbanCards {
			if pc.Pane.PanePath != "" {
				out, err := exec.Command("git", "-C", pc.Pane.PanePath, "rev-parse", "--show-toplevel").Output()
				if err == nil {
					repoRoot = strings.TrimSpace(string(out))
					break
				}
			}
		}
		if repoRoot == "" {
			return kanbanBoardMsg{board: nil}
		}
		board, err := kanban.LoadBoard(repoRoot)
		if err != nil {
			return kanbanBoardMsg{board: nil}
		}
		return kanbanBoardMsg{board: board}
	}
}

func (m *Model) rebuildKanbanView() {
	m.kanbanViewBoard = buildKanbanBoard(m.kanbanBoard, m.kanbanCards)
	m.clampKanbanRow()
}

func (m *Model) clampKanbanRow() {
	colLen := len(m.kanbanViewBoard.Columns[m.selectedCol].Cards)
	if colLen == 0 {
		m.selectedRow = 0
	} else if m.selectedRow >= colLen {
		m.selectedRow = colLen - 1
	}
}

func (m *Model) findPaneByPaneID(paneID string) *tmux.PaneInfo {
	for _, pc := range m.kanbanCards {
		if pc.Pane.PaneID == paneID {
			p := pc.Pane
			return &p
		}
	}
	return nil
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
		m.rebuildGlobalState()
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
		m.rebuildKanbanView()
		return m, pollKanbanBoardCmd(m.kanbanCards)

	case ccStatusMsg:
		m.ccState = msg.state
		return m, nil

	case ccExecDoneMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("CC error: %v", msg.err)
		}
		return m, pollCCStatus

	case kanbanBoardMsg:
		m.kanbanBoard = msg.board
		m.rebuildKanbanView()
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
		return m, tea.Batch(pollSessions, m.pollKanbanCmd(), pollCCStatus, tickCmd())

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
			if m.activeTab == TabKanban {
				if m.selectedCol > 0 {
					m.selectedCol--
					m.clampKanbanRow()
				}
				return m, nil
			}
			if m.activeTab == TabGlobal && m.globalGrouped {
				m.globalCursorPrevGroup()
				return m, nil
			}
		case "right", "l":
			if m.activeTab == TabKanban {
				if m.selectedCol < 2 {
					m.selectedCol++
					m.clampKanbanRow()
				}
				return m, nil
			}
			if m.activeTab == TabGlobal && m.globalGrouped {
				m.globalCursorNextGroup()
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
		case "up", "k":
			if m.activeTab == TabKanban {
				if m.selectedRow > 0 {
					m.selectedRow--
				}
				return m, nil
			}
			if m.activeTab == TabGlobal && m.globalGrouped {
				if m.globalCursor > 0 {
					m.globalCursor--
				}
				return m, nil
			}
		case "down", "j":
			if m.activeTab == TabKanban {
				colLen := len(m.kanbanViewBoard.Columns[m.selectedCol].Cards)
				if m.selectedRow < colLen-1 {
					m.selectedRow++
				}
				return m, nil
			}
			if m.activeTab == TabGlobal && m.globalGrouped {
				if m.globalCursor < len(m.globalItems)-1 {
					m.globalCursor++
				}
				return m, nil
			}
		case "enter":
			if m.activeTab == TabCC {
				return m, m.openCCCmd()
			}
			if m.activeTab == TabGlobal && m.globalGrouped {
				return m, m.handleGroupedEnter()
			}
			if m.activeTab == TabGlobal {
				if _, ok := m.list.SelectedItem().(groupHeaderItem); ok {
					return m, m.handleGroupHeaderEnter()
				}
			}
			return m, m.handleEnter()
		case "g":
			if m.activeTab == TabGlobal {
				m.globalGrouped = !m.globalGrouped
				m.rebuildGlobalState()
				cmd := m.list.SetItems(m.buildGlobalItems())
				return m, cmd
			}
		case "r":
			if m.activeTab == TabGlobal && m.globalGrouped {
				if m.globalCursor < len(m.globalItems) && m.globalItems[m.globalCursor].isHeader {
					item := m.globalItems[m.globalCursor]
					m.renaming = true
					m.renameTarget = item.groupKey
					m.renameInput = item.header.name
					return m, nil
				}
			} else if m.activeTab == TabGlobal {
				if item, ok := m.list.SelectedItem().(groupHeaderItem); ok {
					m.renaming = true
					m.renameTarget = item.key
					m.renameInput = item.name
					return m, nil
				}
			}
		case "p":
			if m.activeTab == TabGlobal && m.globalGrouped {
				if m.globalCursor < len(m.globalItems) && !m.globalItems[m.globalCursor].isHeader {
					pin.Toggle(m.globalItems[m.globalCursor].session.ProjectPath)
					return m, pollSessions
				}
			} else if m.activeTab == TabGlobal {
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

	// Delegate to active list (skip for CC, kanban and grouped global which use custom rendering)
	if m.activeTab == TabCC {
		return m, nil
	}
	if m.activeTab == TabKanban {
		return m, nil
	}
	if m.activeTab == TabGlobal && m.globalGrouped {
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
		col := m.kanbanViewBoard.Columns[m.selectedCol]
		if m.selectedRow < len(col.Cards) {
			card := col.Cards[m.selectedRow]
			// Only jump if the card is in the in-progress column and has a pane ID
			if m.selectedCol == 1 && card.PaneID != "" {
				pane := m.findPaneByPaneID(card.PaneID)
				if pane != nil {
					m.selectedPane = pane
					m.quitting = true
					return tea.Quit
				}
			}
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

func (m *Model) rebuildGlobalState() {
	m.globalItems = buildSelectableItems(m.sessions, m.windowNames, m.collapsedGroups)
	if m.globalCursor >= len(m.globalItems) {
		m.globalCursor = len(m.globalItems) - 1
	}
	if m.globalCursor < 0 {
		m.globalCursor = 0
	}
}

// globalCursorPrevGroup moves the cursor to the previous group header.
func (m *Model) globalCursorPrevGroup() {
	for i := m.globalCursor - 1; i >= 0; i-- {
		if m.globalItems[i].isHeader {
			m.globalCursor = i
			return
		}
	}
}

// globalCursorNextGroup moves the cursor to the next group header.
func (m *Model) globalCursorNextGroup() {
	for i := m.globalCursor + 1; i < len(m.globalItems); i++ {
		if m.globalItems[i].isHeader {
			m.globalCursor = i
			return
		}
	}
}

func (m *Model) handleGroupedEnter() tea.Cmd {
	if m.globalCursor >= len(m.globalItems) {
		return nil
	}
	item := m.globalItems[m.globalCursor]
	if item.isHeader {
		m.collapsedGroups[item.groupKey] = !m.collapsedGroups[item.groupKey]
		m.rebuildGlobalState()
		return nil
	}
	// Session item — jump to pane
	s := item.session
	m.selected = &s
	m.quitting = true
	return tea.Quit
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
	case TabCC:
		h, v := appStyle.GetFrameSize()
		ccHeight := m.height - v - tabBarHeight()
		content = renderCCView(m.ccState, m.width-h, ccHeight)
	case TabKanban:
		h, v := appStyle.GetFrameSize()
		kanbanHeight := m.height - v - tabBarHeight()
		content = renderKanbanColumns(m.kanbanViewBoard, m.selectedCol, m.selectedRow, m.width-h, kanbanHeight)
	case TabGlobal:
		var footer string
		if m.renaming {
			footer = confirmStyle.Render(fmt.Sprintf("Rename: %s█", m.renameInput))
		} else if m.globalGrouped {
			pos := 0
			if len(m.globalItems) > 0 {
				pos = m.globalCursor + 1
			}
			footer = footerStyle.Render(fmt.Sprintf(" %d/%d items  g:flat  r:rename ", pos, len(m.globalItems)))
		} else {
			footer = footerStyle.Render(fmt.Sprintf(" %d/%d sessions  g:grouped ", m.list.Index()+1, m.totalCount))
		}
		if m.globalGrouped {
			h, v := appStyle.GetFrameSize()
			groupedHeight := m.height - v - tabBarHeight() - 2 // footer
			content = renderGroupedGlobal(m.sessions, m.windowNames, m.collapsedGroups, m.globalCursor, m.globalScroll, m.width-h, groupedHeight) + "\n" + footer
		} else {
			content = m.list.View() + "\n" + footer
		}
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

func (m *Model) openCCCmd() tea.Cmd {
	repoRoot := cc.DefaultRepoRoot()

	if _, err := cc.EnsureRunning(repoRoot); err != nil {
		return func() tea.Msg {
			return ccExecDoneMsg{err: fmt.Errorf("CC start failed: %w", err)}
		}
	}

	c := exec.Command("tmux", "-L", "claude-mux-cc", "attach", "-t", "cc")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return ccExecDoneMsg{err: err}
	})
}

func pollCCStatus() tea.Msg {
	state, _ := cc.Status()
	return ccStatusMsg{state: state}
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
