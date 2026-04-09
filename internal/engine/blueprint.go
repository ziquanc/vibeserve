package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

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

// HasPendingBlueprint returns true if a blueprint is awaiting approval.
func (e *Engine) HasPendingBlueprint() bool {
	return e.pendingBlueprint != nil
}

// PendingBlueprint returns the current pending blueprint, or nil.
func (e *Engine) PendingBlueprint() *BlueprintInfo {
	return e.pendingBlueprint
}

// CancelBlueprint discards the pending blueprint.
func (e *Engine) CancelBlueprint() {
	e.pendingBlueprint = nil
}

// proposeBlueprint generates a blueprint from a new manifest without applying it.
func (e *Engine) proposeBlueprint(newManifest *manifest.Manifest) (*BlueprintInfo, error) {
	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestCompleted, Data: newManifest})
	}

	repairManifest(newManifest, e.manifest)

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventManifestGenerated, Data: newManifest})
	}

	// Structural + referential checks are hard gates.
	if err := manifest.ValidateStructure(newManifest); err != nil {
		if e.bus != nil {
			e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
		}
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	// Compilation errors are warnings during proposal — the user can refine.
	var compileWarnings []string
	if err := manifest.ValidateCompilation(newManifest); err != nil {
		compileWarnings = append(compileWarnings, fmt.Sprintf("Script issue: %v (will be fixed on apply)", err))
	}

	changes := manifest.Diff(e.manifest, newManifest)
	if e.bus != nil {
		e.bus.Publish(Event{Type: EventManifestDiffComputed, Data: changes})
	}
	heuristics := ScoreHeuristics(newManifest)
	summary := formatChangeSummaryFromChanges(changes)

	warnings := append([]string{}, compileWarnings...)
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeDropColumn:
			warnings = append(warnings, fmt.Sprintf("Breaking: %s", c.Detail))
		case manifest.ChangeRemoveRoute:
			warnings = append(warnings, fmt.Sprintf("Breaking: %s", c.Detail))
		case manifest.ChangeRemoveScript:
			warnings = append(warnings, fmt.Sprintf("Changed: %s", c.Detail))
		}
	}

	bp := &BlueprintInfo{
		Manifest:   newManifest,
		Changes:    changes,
		Warnings:   warnings,
		Summary:    summary,
		Heuristics: heuristics,
	}

	e.pendingBlueprint = bp
	return bp, nil
}

// ApproveBlueprint applies the pending blueprint.
func (e *Engine) ApproveBlueprint(ctx context.Context) (*ApplyResult, error) {
	if e.pendingBlueprint == nil {
		return nil, fmt.Errorf("no pending blueprint to approve")
	}

	bp := e.pendingBlueprint
	e.pendingBlueprint = nil

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventBlueprintApproved, Data: *bp})
	}

	result := &ApplyResult{}
	return e.applyManifest(ctx, "", bp.Manifest, result)
}

// RefineBlueprint sends feedback to LLM and generates a new blueprint.
func (e *Engine) RefineBlueprint(ctx context.Context, feedback string) (*BlueprintInfo, error) {
	if e.pendingBlueprint == nil {
		return nil, fmt.Errorf("no pending blueprint to refine")
	}

	if e.provider == nil {
		return nil, fmt.Errorf("no LLM provider configured")
	}

	prompt := fmt.Sprintf("Refine the current API design based on this feedback: %s", feedback)

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: "Refining blueprint..."})
	}

	newManifest, err := e.provider.Generate(ctx, e.pendingBlueprint.Manifest, prompt, e.history)
	if err != nil {
		return nil, fmt.Errorf("LLM refinement failed: %w", err)
	}

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestCompleted})
	}

	bp, err := e.proposeBlueprint(newManifest)
	if err != nil {
		return nil, err
	}

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventBlueprintRefined, Data: *bp})
	}

	return bp, nil
}

// FormatBlueprintSummary creates a TUI-friendly summary of a blueprint.
func FormatBlueprintSummary(bp *BlueprintInfo) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Blueprint ready (score: %d/10)\n", bp.Heuristics.Score))
	b.WriteString("\n")

	if bp.Manifest != nil {
		b.WriteString(fmt.Sprintf("  %d tables, %d routes, %d scripts\n",
			len(bp.Manifest.Schemas), len(bp.Manifest.Routes), len(bp.Manifest.Scripts)))
	}

	if len(bp.Warnings) > 0 {
		b.WriteString("\n")
		for _, w := range bp.Warnings {
			b.WriteString(fmt.Sprintf("  \u26a0 %s\n", w))
		}
	}

	if len(bp.Heuristics.Hints) > 0 {
		b.WriteString("\n")
		for _, h := range bp.Heuristics.Hints {
			b.WriteString(fmt.Sprintf("  \u2726 %s\n", h))
		}
	}

	if len(bp.Heuristics.Suggestions) > 0 {
		b.WriteString("\n")
		for _, s := range bp.Heuristics.Suggestions {
			b.WriteString(fmt.Sprintf("  \u26a0 %s\n", s))
		}
	}

	return b.String()
}

// formatChangeSummaryFromChanges creates a human-readable summary from a change list.
func formatChangeSummaryFromChanges(changes []manifest.Change) string {
	var tables, routes, scripts, seeds int
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeAddTable, manifest.ChangeAddColumn, manifest.ChangeDropColumn:
			tables++
		case manifest.ChangeAddRoute, manifest.ChangeUpdateRoute, manifest.ChangeRemoveRoute:
			routes++
		case manifest.ChangeAddScript, manifest.ChangeUpdateScript, manifest.ChangeRemoveScript:
			scripts++
		case manifest.ChangeAddSeed:
			seeds++
		}
	}
	var parts []string
	if tables > 0 {
		parts = append(parts, fmt.Sprintf("%d schema changes", tables))
	}
	if routes > 0 {
		parts = append(parts, fmt.Sprintf("%d route changes", routes))
	}
	if scripts > 0 {
		parts = append(parts, fmt.Sprintf("%d script changes", scripts))
	}
	if seeds > 0 {
		parts = append(parts, fmt.Sprintf("%d seed operations", seeds))
	}
	if len(parts) == 0 {
		return "No changes"
	}
	return strings.Join(parts, ", ")
}
