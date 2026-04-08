package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) (vibeDir, dbPath string) {
	t.Helper()
	tmpDir := t.TempDir()
	vibeDir = filepath.Join(tmpDir, ".vibe")
	os.MkdirAll(vibeDir, 0o755)

	dbPath = filepath.Join(vibeDir, "state.db")
	os.WriteFile(dbPath, []byte("test-db-content-v1"), 0o644)
	return vibeDir, dbPath
}

func TestCreate(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	snap, err := Create(vibeDir, dbPath, "before schema change")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if snap.ID != 1 {
		t.Errorf("expected ID 1, got %d", snap.ID)
	}
	if snap.Description != "before schema change" {
		t.Errorf("expected description 'before schema change', got %q", snap.Description)
	}

	// Verify file exists
	data, err := os.ReadFile(snap.Path)
	if err != nil {
		t.Fatalf("read snapshot file: %v", err)
	}
	if string(data) != "test-db-content-v1" {
		t.Errorf("snapshot content mismatch: %s", data)
	}
}

func TestCreate_AutoIncrement(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	snap1, err := Create(vibeDir, dbPath, "first")
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if snap1.ID != 1 {
		t.Errorf("expected ID 1, got %d", snap1.ID)
	}

	snap2, err := Create(vibeDir, dbPath, "second")
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if snap2.ID != 2 {
		t.Errorf("expected ID 2, got %d", snap2.ID)
	}

	snap3, err := Create(vibeDir, dbPath, "third")
	if err != nil {
		t.Fatalf("Create third: %v", err)
	}
	if snap3.ID != 3 {
		t.Errorf("expected ID 3, got %d", snap3.ID)
	}
}

func TestList_NewestFirst(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	Create(vibeDir, dbPath, "first")
	Create(vibeDir, dbPath, "second")
	Create(vibeDir, dbPath, "third")

	snaps, err := List(vibeDir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}
	if snaps[0].ID != 3 {
		t.Errorf("expected newest first (ID 3), got %d", snaps[0].ID)
	}
	if snaps[1].ID != 2 {
		t.Errorf("expected ID 2 second, got %d", snaps[1].ID)
	}
	if snaps[2].ID != 1 {
		t.Errorf("expected oldest last (ID 1), got %d", snaps[2].ID)
	}
}

func TestList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	// Don't create snapshots dir

	snaps, err := List(vibeDir)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snaps))
	}
}

func TestRestore(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	// Create snapshot of v1
	snap, err := Create(vibeDir, dbPath, "v1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Overwrite db with v2
	os.WriteFile(dbPath, []byte("test-db-content-v2"), 0o644)

	// Verify db is now v2
	data, _ := os.ReadFile(dbPath)
	if string(data) != "test-db-content-v2" {
		t.Fatalf("expected v2, got %s", data)
	}

	// Restore v1
	if err := Restore(snap, dbPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify db is back to v1
	data, _ = os.ReadFile(dbPath)
	if string(data) != "test-db-content-v1" {
		t.Errorf("expected v1 after restore, got %s", data)
	}
}

func TestRestoreLatest(t *testing.T) {
	vibeDir, dbPath := setupTestDB(t)

	Create(vibeDir, dbPath, "first")

	// Change the DB
	os.WriteFile(dbPath, []byte("test-db-content-v2"), 0o644)
	Create(vibeDir, dbPath, "second")

	// Change the DB again
	os.WriteFile(dbPath, []byte("test-db-content-v3"), 0o644)

	// RestoreLatest should restore the "second" snapshot (v2 content)
	snap, err := RestoreLatest(vibeDir, dbPath)
	if err != nil {
		t.Fatalf("RestoreLatest: %v", err)
	}
	if snap.ID != 2 {
		t.Errorf("expected latest snapshot ID 2, got %d", snap.ID)
	}

	data, _ := os.ReadFile(dbPath)
	if string(data) != "test-db-content-v2" {
		t.Errorf("expected v2 content after restore, got %s", data)
	}
}

func TestRestoreLatest_NoSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	vibeDir := filepath.Join(tmpDir, ".vibe")
	dbPath := filepath.Join(vibeDir, "state.db")

	_, err := RestoreLatest(vibeDir, dbPath)
	if err == nil {
		t.Error("expected error when no snapshots exist")
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"before schema change", "before_schema_change"},
		{"Add Users Table!", "add_users_table"},
		{"", "snapshot"},
		{"Hello World 123", "hello_world_123"},
	}
	for _, tt := range tests {
		got := sanitize(tt.input)
		if got != tt.expected {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
