package cc

import _ "embed"

//go:embed system-prompt.md
var systemPrompt string

// SystemPrompt returns the system prompt for the Command Center agent.
func SystemPrompt() string {
	return systemPrompt
}
