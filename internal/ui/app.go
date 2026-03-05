package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mkok/claude-mux/internal/pin"
	"github.com/mkok/claude-mux/internal/session"
)

const pollInterval = 2 * time.Second

// sessionsMsg carries discovered sessions to the TUI.
type sessionsMsg []session.ClaudeSession

// tickMsg triggers a poll for sessions.
type tickMsg time.Time

// Model is the Bubble Tea model for the session list.
type Model struct {
	list       list.Model
	selected   *session.ClaudeSession
	quitting   bool
	width      int
	height     int
	totalCount int
}

// Selected returns the session the user chose, or nil if they quit.
func (m *Model) Selected() *session.ClaudeSession {
	return m.selected
}

// NewModel creates a new TUI model.
func NewModel() *Model {
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

	return &Model{list: l}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(pollSessions, tickCmd())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case sessionsMsg:
		items := make([]list.Item, len(msg))
		// Reserve space for list chrome (borders, padding, cursor prefix)
		itemWidth := m.width - 8
		if itemWidth < 40 {
			itemWidth = 40
		}
		for i, s := range msg {
			items[i] = sessionItem{session: s, maxWidth: itemWidth}
		}
		m.totalCount = len(msg)
		cmd := m.list.SetItems(items)
		return m, cmd

	case tickMsg:
		return m, tea.Batch(pollSessions, tickCmd())

	case tea.KeyMsg:
		// Don't intercept keys while filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				s := item.session
				m.selected = &s
				m.quitting = true
				return m, tea.Quit
			}
		case "p":
			if item, ok := m.list.SelectedItem().(sessionItem); ok {
				pin.Toggle(item.session.ProjectPath)
				return m, pollSessions
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	footer := footerStyle.Render(fmt.Sprintf(" %d/%d sessions ", m.list.Index()+1, m.totalCount))
	return appStyle.Render(m.list.View() + "\n" + footer)
}

func pollSessions() tea.Msg {
	sessions, err := session.DiscoverSessions()
	if err != nil {
		return sessionsMsg(nil)
	}
	return sessionsMsg(sessions)
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
