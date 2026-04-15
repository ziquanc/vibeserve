package cloud

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type localProject struct {
	ID string `json:"id"`
}

// LoadProjectID reads .vibe/project.json in the given vibeDir.
// Returns ("", nil) if the file is missing or malformed (treated as "no cached ID").
// Returns ("", err) for real I/O errors (e.g. permission denied) so callers can surface them.
func LoadProjectID(vibeDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(vibeDir, "project.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read project id: %w", err)
	}
	var p localProject
	if err := json.Unmarshal(data, &p); err != nil {
		return "", nil // malformed treated as absent
	}
	return p.ID, nil
}

// SaveProjectID writes .vibe/project.json with the given ID.
func SaveProjectID(vibeDir, id string) error {
	if err := os.MkdirAll(vibeDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(localProject{ID: id}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(vibeDir, "project.json"), data, 0o644)
}
