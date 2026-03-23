package kanban

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewBoard(t *testing.T) {
	b := NewBoard("swarm-1", "%42")
	if b.SwarmID != "swarm-1" {
		t.Fatalf("expected swarm-1, got %s", b.SwarmID)
	}
	if b.OrchestratorPaneID != "%42" {
		t.Fatalf("expected %%42, got %s", b.OrchestratorPaneID)
	}
	for _, col := range []string{"backlog", "in-progress", "done"} {
		if _, ok := b.Columns[col]; !ok {
			t.Fatalf("missing column %s", col)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kanban.json")

	b := NewBoard("s1", "%1")
	b.Path = path
	b.Columns["backlog"] = []Card{
		{ID: "c1", Title: "Task 1", PaneID: "%10"},
		{ID: "c2", Title: "Task 2", Branch: "feat/x"},
	}

	if err := SaveBoard(b); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadBoardFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.SwarmID != "s1" {
		t.Fatalf("swarm id mismatch: %s", loaded.SwarmID)
	}
	if loaded.Path != path {
		t.Fatalf("path not set")
	}
	if len(loaded.Columns["backlog"]) != 2 {
		t.Fatalf("expected 2 backlog cards, got %d", len(loaded.Columns["backlog"]))
	}
	if loaded.Columns["backlog"][0].ID != "c1" {
		t.Fatalf("card id mismatch")
	}
}

func TestLoadBoardGlob(t *testing.T) {
	root := t.TempDir()

	// Create two swarm dirs; the second is written later so it should be picked
	dir1 := filepath.Join(root, ".claude-mux", "swarm-aaa")
	dir2 := filepath.Join(root, ".claude-mux", "swarm-bbb")
	os.MkdirAll(dir1, 0o755)
	os.MkdirAll(dir2, 0o755)

	b1 := NewBoard("aaa", "%1")
	b1.Path = filepath.Join(dir1, "kanban.json")
	SaveBoard(b1)

	b2 := NewBoard("bbb", "%2")
	b2.Path = filepath.Join(dir2, "kanban.json")
	SaveBoard(b2)

	loaded, err := LoadBoard(root)
	if err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}
	// Should load the most recently modified one (bbb, written second)
	if loaded.SwarmID != "bbb" {
		t.Fatalf("expected bbb, got %s", loaded.SwarmID)
	}
}

func TestLoadBoardNotFound(t *testing.T) {
	_, err := LoadBoard(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing board")
	}
}

func TestMoveCard(t *testing.T) {
	b := NewBoard("s", "%1")
	b.Columns["backlog"] = []Card{
		{ID: "c1", Title: "T1"},
		{ID: "c2", Title: "T2"},
	}

	if err := MoveCard(b, "c1", "in-progress"); err != nil {
		t.Fatalf("move: %v", err)
	}

	if len(b.Columns["backlog"]) != 1 {
		t.Fatalf("expected 1 backlog card, got %d", len(b.Columns["backlog"]))
	}
	if b.Columns["backlog"][0].ID != "c2" {
		t.Fatalf("wrong card remaining in backlog")
	}
	if len(b.Columns["in-progress"]) != 1 || b.Columns["in-progress"][0].ID != "c1" {
		t.Fatalf("card not moved to in-progress")
	}

	// Move to done
	if err := MoveCard(b, "c1", "done"); err != nil {
		t.Fatalf("move to done: %v", err)
	}
	if len(b.Columns["in-progress"]) != 0 {
		t.Fatalf("in-progress should be empty")
	}
	if len(b.Columns["done"]) != 1 {
		t.Fatalf("done should have 1 card")
	}
}

func TestMoveCardNotFound(t *testing.T) {
	b := NewBoard("s", "%1")
	err := MoveCard(b, "nonexistent", "done")
	if err == nil {
		t.Fatal("expected error for missing card")
	}
}

func TestMoveCardInvalidColumn(t *testing.T) {
	b := NewBoard("s", "%1")
	b.Columns["backlog"] = []Card{{ID: "c1"}}
	err := MoveCard(b, "c1", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid column")
	}
}

func TestFindCardByPaneID(t *testing.T) {
	b := NewBoard("s", "%1")
	b.Columns["in-progress"] = []Card{
		{ID: "c1", PaneID: "%10"},
		{ID: "c2", PaneID: "%20"},
	}

	card, col := FindCardByPaneID(b, "%10")
	if card == nil {
		t.Fatal("expected to find card")
	}
	if card.ID != "c1" {
		t.Fatalf("wrong card: %s", card.ID)
	}
	if col != "in-progress" {
		t.Fatalf("wrong column: %s", col)
	}

	card, col = FindCardByPaneID(b, "%99")
	if card != nil {
		t.Fatal("expected nil for missing pane")
	}
}

func TestLoadBoardInitializesColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kanban.json")
	// Write a board with only one column
	os.WriteFile(path, []byte(`{"swarm_id":"s","columns":{"backlog":[]}}`), 0o644)

	b, err := LoadBoardFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, col := range []string{"backlog", "in-progress", "done"} {
		if _, ok := b.Columns[col]; !ok {
			t.Fatalf("missing column %s after load", col)
		}
	}
}
