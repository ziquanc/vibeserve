package cloud

import (
	"encoding/json"
	"fmt"
)

type SyncInput struct {
	Name       string `json:"name"`
	TableCount int    `json:"tableCount"`
	RouteCount int    `json:"routeCount"`
	Status     string `json:"status,omitempty"`
}

type projectResp struct {
	Project struct {
		ID string `json:"id"`
	} `json:"project"`
}

// SyncProject upserts a project to the platform.
// If .vibe/project.json has an ID, it PATCHes; otherwise POSTs and saves the new ID.
// Returns the project ID (server-assigned or from the local cache).
func SyncProject(c *Client, vibeDir string, in SyncInput) (string, error) {
	existingID := LoadProjectID(vibeDir)

	var resp *projectResp
	var err error

	if existingID != "" {
		resp, err = patchProject(c, existingID, in)
		if err != nil {
			return "", fmt.Errorf("patch project: %w", err)
		}
	} else {
		resp, err = postProject(c, in)
		if err != nil {
			return "", fmt.Errorf("post project: %w", err)
		}
		if err := SaveProjectID(vibeDir, resp.Project.ID); err != nil {
			return "", fmt.Errorf("save project id: %w", err)
		}
	}

	return resp.Project.ID, nil
}

func postProject(c *Client, in SyncInput) (*projectResp, error) {
	httpResp, err := c.Post("/api/projects", in)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var out projectResp
	if err := json.NewDecoder(httpResp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &out, nil
}

func patchProject(c *Client, id string, in SyncInput) (*projectResp, error) {
	httpResp, err := c.Patch("/api/projects/"+id, in)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var out projectResp
	if err := json.NewDecoder(httpResp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &out, nil
}
