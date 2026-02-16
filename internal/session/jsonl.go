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
