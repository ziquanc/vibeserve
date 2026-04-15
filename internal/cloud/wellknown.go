package cloud

import "github.com/vibeserve/vibeserve/internal/manifest"

// WellKnown is the shape served at /.well-known/vibeserve.json.
// It describes a deployed VibeServe API for AI agents and external tools.
//
// Phase 5 (directory) will add `type`, `location`, `capabilities` —
// for now we publish what the manifest already knows.
type WellKnown struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Endpoints   []WellKnownEndpoint `json:"endpoints"`
	OpenAPI     string              `json:"openapi"`
}

type WellKnownEndpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// GenerateWellKnown projects a Manifest into the .well-known descriptor.
func GenerateWellKnown(m *manifest.Manifest) WellKnown {
	endpoints := make([]WellKnownEndpoint, 0, len(m.Routes))
	for _, r := range m.Routes {
		endpoints = append(endpoints, WellKnownEndpoint{
			Method: r.Method,
			Path:   r.Path,
		})
	}
	return WellKnown{
		Name:        m.Name,
		Description: m.Description,
		Endpoints:   endpoints,
		OpenAPI:     "/openapi.yaml",
	}
}
