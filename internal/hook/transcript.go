package hook

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

type transcriptEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type messageContent struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// lastAssistantText reads the tail of a transcript JSONL file and returns
// the text content of the last assistant message, truncated for display.
func lastAssistantText(path string, tailBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	offset := info.Size() - tailBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")

	// Walk backwards to find the last assistant entry with text content
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		var entry transcriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" {
			continue
		}

		var msg messageContent
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		// Content can be an array of blocks
		var blocks []contentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}

		// Find the last text block (the final thing Claude said)
		var lastText string
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				lastText = b.Text
			}
		}

		if lastText != "" {
			return extractQuestion(lastText), nil
		}
	}

	return "", nil
}

// extractQuestion tries to pull out the key question or final statement
// from Claude's response for display in the session list.
func extractQuestion(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	// Walk backwards from the end to find a question or meaningful line
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Skip markdown formatting lines
		if line == "```" || line == "---" || line == "```json" {
			continue
		}
		// If it's a question, use it
		if strings.HasSuffix(line, "?") {
			return truncate(line, 120)
		}
		// Otherwise use the last non-empty line
		return truncate(line, 120)
	}

	return truncate(text, 120)
}
