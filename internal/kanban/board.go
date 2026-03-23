package kanban

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var validColumns = map[string]bool{
	"backlog":     true,
	"in-progress": true,
	"done":        true,
}

type Card struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	PaneID      string `json:"pane_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
}

type Board struct {
	SwarmID            string            `json:"swarm_id"`
	OrchestratorPaneID string            `json:"orchestrator_pane_id"`
	CreatedAt          time.Time         `json:"created_at"`
	Path               string            `json:"-"`
	Columns            map[string][]Card `json:"columns"`
}

func NewBoard(swarmID, orchPaneID string) *Board {
	return &Board{
		SwarmID:            swarmID,
		OrchestratorPaneID: orchPaneID,
		CreatedAt:          time.Now(),
		Columns: map[string][]Card{
			"backlog":     {},
			"in-progress": {},
			"done":        {},
		},
	}
}

func LoadBoard(repoRoot string) (*Board, error) {
	pattern := filepath.Join(repoRoot, ".claude-mux", "swarm-*", "kanban.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob kanban files: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no kanban board found in %s", repoRoot)
	}

	var newest string
	var newestTime time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest = m
			newestTime = info.ModTime()
		}
	}

	return LoadBoardFromPath(newest)
}

func LoadBoardFromPath(path string) (*Board, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read board: %w", err)
	}

	var board Board
	if err := json.Unmarshal(data, &board); err != nil {
		return nil, fmt.Errorf("parse board: %w", err)
	}
	board.Path = path

	// Ensure all three columns exist
	for col := range validColumns {
		if board.Columns == nil {
			board.Columns = make(map[string][]Card)
		}
		if _, ok := board.Columns[col]; !ok {
			board.Columns[col] = []Card{}
		}
	}

	return &board, nil
}

func SaveBoard(board *Board) error {
	if board.Path == "" {
		return fmt.Errorf("board has no path set")
	}

	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}

	tmpPath := board.Path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(board.Path), 0o755); err != nil {
		return fmt.Errorf("create board directory: %w", err)
	}
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp board: %w", err)
	}
	if err := os.Rename(tmpPath, board.Path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename board: %w", err)
	}

	return nil
}

func MoveCard(board *Board, cardID, toColumn string) error {
	if !validColumns[toColumn] {
		return fmt.Errorf("invalid column: %q", toColumn)
	}

	for col, cards := range board.Columns {
		for i, c := range cards {
			if c.ID == cardID {
				board.Columns[col] = append(cards[:i], cards[i+1:]...)
				board.Columns[toColumn] = append(board.Columns[toColumn], c)
				return nil
			}
		}
	}

	return fmt.Errorf("card not found: %q", cardID)
}

func FindCardByID(board *Board, cardID string) (*Card, string) {
	for col, cards := range board.Columns {
		for i := range cards {
			if cards[i].ID == cardID {
				return &cards[i], col
			}
		}
	}
	return nil, ""
}

func FindCardByPaneID(board *Board, paneID string) (*Card, string) {
	for col, cards := range board.Columns {
		for i := range cards {
			if cards[i].PaneID == paneID {
				return &cards[i], col
			}
		}
	}
	return nil, ""
}
