package web

import (
	"fmt"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/export"
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
	Steps      []string       `json:"steps,omitempty"`
	Prompt     string         `json:"prompt,omitempty"`
	Changes    []changeInfo   `json:"changes,omitempty"`
	Diagram    string         `json:"diagram,omitempty"`
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
		// No pending blueprint — show current manifest as "active" if one exists
		m := bh.engine.Manifest()
		if m != nil && len(m.Routes) > 0 {
			writeConsoleJSON(w, http.StatusOK, blueprintResponse{
				Status:   "active",
				Manifest: m,
			})
			return
		}
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
		Steps:    bp.Steps,
		Prompt:   bp.Prompt,
		Changes:  changes,
		Diagram:  bp.Diagram,
		Heuristics: &heuristicInfo{
			Score:       bp.Heuristics.Score,
			Hints:       bp.Heuristics.Hints,
			Suggestions: bp.Heuristics.Suggestions,
		},
		Warnings: bp.Warnings,
	}

	writeConsoleJSON(w, http.StatusOK, resp)
}

// HandleDiagram serves GET /_blueprint/diagram — fullscreen ER diagram page.
func (bh *BlueprintHandler) HandleDiagram(w http.ResponseWriter, r *http.Request) {
	// Get diagram from pending blueprint or generate from current manifest.
	var diagram string
	if bp := bh.engine.PendingBlueprint(); bp != nil && bp.Diagram != "" {
		diagram = bp.Diagram
	} else if m := bh.engine.Manifest(); m != nil && len(m.Schemas) > 0 {
		diagram = manifest.GenerateMermaidER(m.Schemas)
	}

	if diagram == "" {
		http.Error(w, "No schema available for diagram.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>ER Diagram — VibeServe</title>
<script src="https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js"></script>
<style>
body { background: #111; margin: 0; padding: 40px; display: flex; justify-content: center; align-items: flex-start; min-height: 100vh; }
.mermaid { max-width: 100%%; }
.mermaid svg { max-width: 100%%; height: auto; }
</style>
</head><body>
<pre class="mermaid">%s</pre>
<script>
mermaid.initialize({
  startOnLoad: true,
  theme: 'dark',
  themeVariables: {
    primaryColor: '#7C3AED',
    primaryTextColor: '#E5E7EB',
    lineColor: '#6B7280',
    secondaryColor: '#1F2937',
    tertiaryColor: '#374151'
  }
});
</script>
</body></html>`, diagram)
}

// HandleOpenAPI serves GET /_api/openapi.yaml — generates OpenAPI spec from current manifest.
func (bh *BlueprintHandler) HandleOpenAPI(w http.ResponseWriter, r *http.Request) {
	m := bh.engine.Manifest()
	if m == nil || len(m.Routes) == 0 {
		http.Error(w, "No manifest loaded. Create an API first.", http.StatusNotFound)
		return
	}

	yaml := export.GenerateOpenAPI(m)
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(yaml))
}
