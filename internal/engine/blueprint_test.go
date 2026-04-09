package engine

import (
	"testing"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestEngine_HasPendingBlueprint(t *testing.T) {
	eng := &Engine{}
	if eng.HasPendingBlueprint() {
		t.Error("should not have pending blueprint initially")
	}
	eng.pendingBlueprint = &BlueprintInfo{Manifest: &manifest.Manifest{Name: "test"}}
	if !eng.HasPendingBlueprint() {
		t.Error("should have pending blueprint after setting")
	}
}

func TestEngine_CancelBlueprint(t *testing.T) {
	eng := &Engine{}
	eng.pendingBlueprint = &BlueprintInfo{Manifest: &manifest.Manifest{Name: "test"}}
	eng.CancelBlueprint()
	if eng.HasPendingBlueprint() {
		t.Error("should not have pending blueprint after cancel")
	}
}

func TestEngine_PendingBlueprint(t *testing.T) {
	eng := &Engine{}
	if eng.PendingBlueprint() != nil {
		t.Error("should return nil when no pending blueprint")
	}
	bp := &BlueprintInfo{
		Manifest:   &manifest.Manifest{Name: "test"},
		Heuristics: HeuristicResult{Score: 5},
	}
	eng.pendingBlueprint = bp
	got := eng.PendingBlueprint()
	if got == nil || got.Manifest.Name != "test" || got.Heuristics.Score != 5 {
		t.Error("should return correct pending blueprint")
	}
}

func TestFormatBlueprintSummary(t *testing.T) {
	bp := &BlueprintInfo{
		Manifest: &manifest.Manifest{
			Schemas: []manifest.Schema{{Table: "a"}, {Table: "b"}},
			Routes:  []manifest.Route{{Path: "/a"}, {Path: "/b"}},
			Scripts: []manifest.Script{{Name: "s1"}, {Name: "s2"}},
		},
		Heuristics: HeuristicResult{
			Score: 6,
			Hints: []string{"State transition: POST /a/:id/activate"},
		},
	}
	summary := FormatBlueprintSummary(bp)
	if summary == "" {
		t.Error("summary should not be empty")
	}
	if !contains(summary, "score: 6/10") {
		t.Error("should contain score")
	}
	if !contains(summary, "2 routes") {
		t.Error("should contain route count")
	}
	if !contains(summary, "2 tables") {
		t.Error("should contain table count")
	}
}

