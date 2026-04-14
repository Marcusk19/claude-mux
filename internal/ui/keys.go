package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit         key.Binding
	Select       key.Binding
	Pin          key.Binding
	Tab          key.Binding
	ShiftTab     key.Binding
	Remove       key.Binding
	Group        key.Binding
	Rename       key.Binding
	ToggleSelect key.Binding
	Broadcast    key.Binding
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
		key.WithHelp("tab", "next tab"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev tab"),
	),
	Remove: key.NewBinding(
		key.WithKeys("d", "x"),
		key.WithHelp("d/x", "remove worktree"),
	),
	Group: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "group/ungroup"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename group"),
	),
	ToggleSelect: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "select"),
	),
	Broadcast: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "broadcast"),
	),
}
