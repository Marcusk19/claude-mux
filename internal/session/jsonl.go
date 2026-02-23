package session

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

type jsonlEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
}


type promptEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type promptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type promptContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// readFirstUserPrompt reads the beginning of a JSONL transcript and returns
// the text of the first human message, truncated for display.
func readFirstUserPrompt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Read up to 16KB from the start — the first user message should be near the top.
	buf := make([]byte, 16384)
	n, _ := f.Read(buf)
	if n == 0 {
		return ""
	}

	lines := strings.Split(string(buf[:n]), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry promptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "user" {
			continue
		}
		var msg promptMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}
		// Content can be a string or an array of blocks.
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			text = strings.TrimSpace(text)
			if text != "" {
				return truncatePrompt(text, 120)
			}
			continue
		}
		var blocks []promptContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				return truncatePrompt(strings.TrimSpace(b.Text), 120)
			}
		}
	}
	return ""
}

func truncatePrompt(s string, max int) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// readJSONLTail reads the last tailBytes of a JSONL file and returns the
// timestamp and type of the last entry that has both fields.
func readJSONLTail(path string, tailBytes int64) (lastTime time.Time, lastType string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return time.Time{}, "", err
	}

	offset := info.Size() - tailBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return time.Time{}, "", err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return time.Time{}, "", err
	}

	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry jsonlEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			continue
		}
		return t, entry.Type, nil
	}

	return time.Time{}, "", os.ErrNotExist
}
