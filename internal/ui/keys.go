package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit   key.Binding
	Select key.Binding
	Pin    key.Binding
	Tab    key.Binding
	Remove key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "esc"),
		key.WithHelp("q/esc", "quit"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "jump"),
	),
	Pin: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pin/unpin"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	Remove: key.NewBinding(
		key.WithKeys("d", "x"),
		key.WithHelp("d/x", "remove worktree"),
	),
}
