package engine

import "github.com/vibeserve/vibeserve/internal/manifest"

// HeuristicResult holds the architectural quality analysis of a blueprint.
type HeuristicResult struct {
	Score       int      // 0-10, higher = more architectural depth
	Hints       []string // non-CRUD patterns detected (shown as highlights)
	Suggestions []string // improvements the user could request
}

// BlueprintInfo holds a proposed manifest with its analysis.
type BlueprintInfo struct {
	Manifest   *manifest.Manifest
	Changes    []manifest.Change
	Warnings   []string
	Summary    string
	Heuristics HeuristicResult
}

// BlueprintResult is returned by Apply() in proposal mode.
type BlueprintResult struct {
	Blueprint    *BlueprintInfo // non-nil when a proposal is pending
	ChatResponse string         // non-empty if LLM responded conversationally
}
