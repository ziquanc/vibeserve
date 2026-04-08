package snapshot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Snapshot represents a saved database snapshot.
type Snapshot struct {
	ID          int
	Description string
	Path        string
}

// Dir returns the snapshots directory for a given vibe directory.
func Dir(vibeDir string) string {
	return filepath.Join(vibeDir, "snapshots")
}

// Create copies the database file at dbPath into the snapshots directory
// with an auto-incrementing ID and the given description.
// Returns the created Snapshot.
func Create(vibeDir, dbPath, description string) (*Snapshot, error) {
	snapDir := Dir(vibeDir)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}

	// Find the next ID
	nextID, err := nextID(snapDir)
	if err != nil {
		return nil, fmt.Errorf("next snapshot id: %w", err)
	}

	// Sanitize description for filename
	safeName := sanitize(description)
	filename := fmt.Sprintf("%03d_%s.db", nextID, safeName)
	destPath := filepath.Join(snapDir, filename)

	if err := copyFile(dbPath, destPath); err != nil {
		return nil, fmt.Errorf("copy snapshot: %w", err)
	}

	return &Snapshot{
		ID:          nextID,
		Description: description,
		Path:        destPath,
	}, nil
}

// Restore copies a snapshot file back to the database path, overwriting it.
func Restore(snap *Snapshot, dbPath string) error {
	if err := copyFile(snap.Path, dbPath); err != nil {
		return fmt.Errorf("restore snapshot %d: %w", snap.ID, err)
	}
	return nil
}

// RestoreLatest finds the most recent snapshot and restores it.
// Returns the restored snapshot, or an error if no snapshots exist.
func RestoreLatest(vibeDir, dbPath string) (*Snapshot, error) {
	snaps, err := List(vibeDir)
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no snapshots found")
	}
	latest := snaps[0] // List returns newest-first
	if err := Restore(&latest, dbPath); err != nil {
		return nil, err
	}
	return &latest, nil
}

// List returns all snapshots in the vibe directory, sorted newest-first (highest ID first).
func List(vibeDir string) ([]Snapshot, error) {
	snapDir := Dir(vibeDir)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshot dir: %w", err)
	}

	var snaps []Snapshot
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		snap, ok := parseFilename(name, snapDir)
		if !ok {
			continue
		}
		snaps = append(snaps, snap)
	}

	// Sort newest-first
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].ID > snaps[j].ID
	})

	return snaps, nil
}

// snapshotPattern matches filenames like "001_description.db"
var snapshotPattern = regexp.MustCompile(`^(\d{3})_(.+)\.db$`)

func parseFilename(name, dir string) (Snapshot, bool) {
	matches := snapshotPattern.FindStringSubmatch(name)
	if matches == nil {
		return Snapshot{}, false
	}
	id, err := strconv.Atoi(matches[1])
	if err != nil {
		return Snapshot{}, false
	}
	return Snapshot{
		ID:          id,
		Description: matches[2],
		Path:        filepath.Join(dir, name),
	}, true
}

func nextID(snapDir string) (int, error) {
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}

	maxID := 0
	for _, entry := range entries {
		snap, ok := parseFilename(entry.Name(), snapDir)
		if ok && snap.ID > maxID {
			maxID = snap.ID
		}
	}
	return maxID + 1, nil
}

// sanitize turns a description into a safe filename component.
func sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, s)
	if len(s) > 50 {
		s = s[:50]
	}
	if s == "" {
		s = "snapshot"
	}
	return s
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
