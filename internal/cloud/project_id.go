package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type localProject struct {
	ID string `json:"id"`
}

// LoadProjectID reads .vibe/project.json in the given vibeDir.
// Returns "" if the file doesn't exist or is malformed.
func LoadProjectID(vibeDir string) string {
	data, err := os.ReadFile(filepath.Join(vibeDir, "project.json"))
	if err != nil {
		return ""
	}
	var p localProject
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.ID
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
