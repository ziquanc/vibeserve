package manifest

import (
	"encoding/json"
	"os"
)

type Manifest struct {
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Database    string   `json:"database,omitempty"`
	Schemas     []Schema `json:"schemas"`
	Routes      []Route  `json:"routes"`
	Scripts     []Script `json:"scripts"`
	Seeds       []Seed   `json:"seeds"`
}

type Schema struct {
	Table        string        `json:"table"`
	Columns      []Column      `json:"columns"`
	StateMachine *StateMachine `json:"state_machine,omitempty"`
}

type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Primary    bool   `json:"primary,omitempty"`
	Auto       bool   `json:"auto,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Unique     bool   `json:"unique,omitempty"`
	Default    any    `json:"default,omitempty"`
	References string `json:"references,omitempty"`
}

type Route struct {
	Path         string            `json:"path"`
	Method       string            `json:"method"`
	Description  string            `json:"description"`
	Script       string            `json:"script"`
	RequestBody  map[string]string `json:"request_body,omitempty"`
	ResponseType string            `json:"response_type"`
}

type Script struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type Seed struct {
	Table string           `json:"table"`
	Rows  []map[string]any `json:"rows"`
}

// LoadFromFile reads and parses a manifest JSON file.
func LoadFromFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
