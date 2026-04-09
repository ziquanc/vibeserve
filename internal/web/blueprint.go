package web

import (
	"net/http"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// BlueprintHandler serves the blueprint preview API.
type BlueprintHandler struct {
	engine *engine.Engine
}

// NewBlueprintHandler creates a BlueprintHandler.
func NewBlueprintHandler(eng *engine.Engine) *BlueprintHandler {
	return &BlueprintHandler{engine: eng}
}

type blueprintResponse struct {
	Status     string         `json:"status"`
	Manifest   any            `json:"manifest,omitempty"`
	Changes    []changeInfo   `json:"changes,omitempty"`
	Heuristics *heuristicInfo `json:"heuristics,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
}

type changeInfo struct {
	Type     string `json:"type"`
	Detail   string `json:"detail"`
	Breaking bool   `json:"breaking"`
}

type heuristicInfo struct {
	Score       int      `json:"score"`
	Hints       []string `json:"hints"`
	Suggestions []string `json:"suggestions"`
}

// HandleBlueprint serves GET /_api/blueprint.
func (bh *BlueprintHandler) HandleBlueprint(w http.ResponseWriter, r *http.Request) {
	bp := bh.engine.PendingBlueprint()
	if bp == nil {
		writeConsoleJSON(w, http.StatusOK, blueprintResponse{Status: "none"})
		return
	}

	var changes []changeInfo
	for _, c := range bp.Changes {
		breaking := c.Type == manifest.ChangeDropColumn || c.Type == manifest.ChangeRemoveRoute
		changes = append(changes, changeInfo{
			Type:     string(c.Type),
			Detail:   c.Detail,
			Breaking: breaking,
		})
	}

	resp := blueprintResponse{
		Status:   "pending",
		Manifest: bp.Manifest,
		Changes:  changes,
		Heuristics: &heuristicInfo{
			Score:       bp.Heuristics.Score,
			Hints:       bp.Heuristics.Hints,
			Suggestions: bp.Heuristics.Suggestions,
		},
		Warnings: bp.Warnings,
	}

	writeConsoleJSON(w, http.StatusOK, resp)
}
