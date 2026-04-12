package engine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/vibeserve/vibeserve/internal/llm"
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
	Diagram    string // Mermaid ER diagram

	// Plan-mode fields: when the LLM returns a multi-step plan,
	// the blueprint stores the steps and defers execution until approval.
	Steps  []string // non-empty when this is a plan (no manifest yet)
	Prompt string   // original user prompt (needed for step execution on approve)
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
		Diagram:    manifest.GenerateMermaidER(newManifest.Schemas),
	}

	e.pendingBlueprint = bp
	return bp, nil
}

// ApproveBlueprint applies the pending blueprint.
// If the blueprint has Steps (plan-mode), executes steps via LLM first.
// If it has a Manifest (direct-mode), applies immediately.
func (e *Engine) ApproveBlueprint(ctx context.Context) (*ApplyResult, error) {
	if e.pendingBlueprint == nil {
		return nil, fmt.Errorf("no pending blueprint to approve")
	}

	bp := e.pendingBlueprint
	e.pendingBlueprint = nil

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventBlueprintApproved, Data: *bp})
	}

	// If we have a complete manifest (with routes + scripts), apply directly.
	if bp.Manifest != nil && len(bp.Manifest.Routes) > 0 {
		result := &ApplyResult{}
		return e.applyManifest(ctx, bp.Prompt, bp.Manifest, result)
	}

	// Schema preview was approved — now generate the FULL manifest.
	// This does a single LLM call to produce routes, scripts, and seeds.
	if bp.Prompt != "" {
		if e.bus != nil {
			e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: "Generating full API..."})
		}
		newManifest, err := e.provider.Generate(ctx, e.manifest, bp.Prompt, e.history)
		if err != nil {
			return nil, fmt.Errorf("full generation failed: %w", err)
		}
		result := &ApplyResult{}
		return e.applyManifest(ctx, bp.Prompt, newManifest, result)
	}

	// Legacy: plan-mode step execution.
	if len(bp.Steps) > 0 {
		return e.executePlanSteps(ctx, bp.Steps, bp.Prompt)
	}

	return nil, fmt.Errorf("blueprint has neither manifest nor steps")
}

// executePlanSteps runs LLM for each step, accumulates the manifest, then applies.
func (e *Engine) executePlanSteps(ctx context.Context, steps []string, prompt string) (*ApplyResult, error) {
	currentManifest := e.manifest
	var finalManifest *manifest.Manifest
	result := &ApplyResult{}

	for i, step := range steps {
		stepNum := i + 1
		log.Printf("[engine] executing step %d/%d: %s", stepNum, len(steps), step)
		if e.bus != nil {
			e.bus.Publish(Event{Type: EventStepStarted, Data: StepInfo{
				Index: stepNum, Total: len(steps), Description: step,
			}})
		}

		stepPrompt := fmt.Sprintf("Implement step %d of %d: %s\n\nIMPORTANT: Output ONLY the complete updated manifest JSON. Include ALL existing tables, routes, scripts, and seeds plus the new additions for this step.", stepNum, len(steps), step)

		newManifest, err := e.provider.Generate(ctx, currentManifest, stepPrompt, e.history)
		if err != nil {
			if chatErr, ok := err.(*llm.ChatOnlyError); ok {
				log.Printf("[engine] step %d returned text instead of JSON, skipping: %s", stepNum, chatErr.Text[:min(100, len(chatErr.Text))])
				if e.bus != nil {
					e.bus.Publish(Event{Type: EventStepCompleted, Data: StepInfo{
						Index: stepNum, Total: len(steps), Description: step,
						Changes: []string{"skipped (text response)"},
					}})
				}
				continue
			}
			return nil, fmt.Errorf("step %d/%d failed: %w", stepNum, len(steps), err)
		}

		// Merge: keep everything from current manifest, add/update from LLM output.
		// This prevents the LLM from silently dropping tables/routes.
		if currentManifest != nil {
			newManifest = mergeManifests(currentManifest, newManifest)
		}

		// Apply the merged manifest
		stepResult, err := e.applyManifest(ctx, step, newManifest, &ApplyResult{})
		if err != nil {
			log.Printf("[engine] step %d failed to apply: %v", stepNum, err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("Step %d failed: %v", stepNum, err))
			if e.bus != nil {
				e.bus.Publish(Event{Type: EventStepCompleted, Data: StepInfo{
					Index: stepNum, Total: len(steps), Description: step,
					Changes: []string{fmt.Sprintf("failed: %v", err)},
				}})
			}
			continue
		}

		currentManifest = stepResult.Manifest
		finalManifest = stepResult.Manifest
		result.Changes = append(result.Changes, stepResult.Changes...)
		result.Warnings = append(result.Warnings, stepResult.Warnings...)
		result.PendingSeeds = append(result.PendingSeeds, stepResult.PendingSeeds...)
		result.Manifest = stepResult.Manifest

		var changeSummaries []string
		for _, c := range stepResult.Changes {
			changeSummaries = append(changeSummaries, c.Detail)
		}
		if e.bus != nil {
			e.bus.Publish(Event{Type: EventStepCompleted, Data: StepInfo{
				Index: stepNum, Total: len(steps), Description: step,
				Changes: changeSummaries,
			}})
		}
	}

	if finalManifest == nil {
		return nil, fmt.Errorf("all plan steps failed to produce a manifest")
	}

	return result, nil
}

// RefineBlueprint sends feedback to LLM and generates a new blueprint.
// In plan-mode (steps only, no manifest), regenerates the plan with feedback.
// In manifest-mode, regenerates the manifest with feedback.
func (e *Engine) RefineBlueprint(ctx context.Context, feedback string) (*BlueprintInfo, error) {
	if e.pendingBlueprint == nil {
		return nil, fmt.Errorf("no pending blueprint to refine")
	}

	if e.provider == nil {
		return nil, fmt.Errorf("no LLM provider configured")
	}

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: "Refining blueprint..."})
	}

	// Plan-mode: regenerate the plan with the original prompt + feedback
	if len(e.pendingBlueprint.Steps) > 0 {
		return e.refinePlan(ctx, feedback)
	}

	// Manifest-mode: ask LLM to refine the existing manifest
	prompt := fmt.Sprintf("Refine the current API design based on this feedback: %s\n\nIMPORTANT: Keep ALL existing tables, routes, scripts, and seeds. Only ADD or MODIFY based on the feedback. Do NOT remove anything unless explicitly asked.", feedback)

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

// refinePlan keeps existing steps and asks the LLM only for NEW steps to add.
// Original steps are preserved in code — the LLM cannot remove them.
func (e *Engine) refinePlan(ctx context.Context, feedback string) (*BlueprintInfo, error) {
	oldSteps := e.pendingBlueprint.Steps
	originalPrompt := e.pendingBlueprint.Prompt

	// Build existing steps summary for context
	var stepsText strings.Builder
	for i, s := range oldSteps {
		stepsText.WriteString(fmt.Sprintf("%d. %s\n", i+1, s))
	}

	// Ask LLM ONLY for the new steps — we merge them ourselves
	directPrompt := fmt.Sprintf(`The user is building: "%s"

These steps are ALREADY planned and will be executed:
%s
The user now wants to ADD: "%s"

Output ONLY a JSON array of the NEW additional steps needed for this enhancement.
Do NOT repeat the existing steps above — only output what's NEW.
Keep the new steps specific and actionable.`, originalPrompt, stepsText.String(), feedback)

	planManifest, planErr := e.provider.Generate(ctx, e.manifest, directPrompt, nil)

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestCompleted})
	}

	var steps []string
	if planErr != nil {
		if chatErr, ok := planErr.(*llm.ChatOnlyError); ok {
			parsed, parseErr := llm.ExtractPlan(chatErr.Text)
			if parseErr == nil && len(parsed) > 0 {
				steps = parsed
			} else {
				return nil, fmt.Errorf("failed to parse refined plan: %w", parseErr)
			}
		} else {
			return nil, fmt.Errorf("plan refinement failed: %w", planErr)
		}
	}

	// If LLM returned a manifest directly, try to extract steps from it
	if planManifest != nil && len(steps) == 0 {
		// LLM gave a manifest instead of a plan — propose it directly
		bp, err := e.proposeBlueprint(planManifest)
		if err != nil {
			return nil, err
		}
		bp.Prompt = originalPrompt
		if e.bus != nil {
			e.bus.Publish(Event{Type: EventBlueprintRefined, Data: *bp})
		}
		return bp, nil
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("refinement produced no new steps")
	}

	// Merge: original steps + new steps from feedback
	mergedSteps := make([]string, 0, len(oldSteps)+len(steps))
	mergedSteps = append(mergedSteps, oldSteps...)
	mergedSteps = append(mergedSteps, steps...)

	bp := &BlueprintInfo{
		Steps:   mergedSteps,
		Prompt:  originalPrompt,
		Summary: fmt.Sprintf("Refined plan: %d steps (%d original + %d new)", len(mergedSteps), len(oldSteps), len(steps)),
	}
	e.pendingBlueprint = bp

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventBlueprintRefined, Data: *bp})
		e.bus.Publish(Event{Type: EventPlanCreated, Data: PlanInfo{Steps: steps, Total: len(steps)}})
	}

	return bp, nil
}

// mergeManifests combines old and new manifests. Everything in old is kept.
// New items are added. If both have the same table/route/script, new wins.
// This prevents LLM from silently dropping entities.
func mergeManifests(old, new *manifest.Manifest) *manifest.Manifest {
	merged := &manifest.Manifest{
		Version:     new.Version,
		Name:        new.Name,
		Description: new.Description,
	}
	if merged.Name == "" {
		merged.Name = old.Name
	}
	if merged.Version == "" {
		merged.Version = old.Version
	}
	if merged.Description == "" {
		merged.Description = old.Description
	}

	// Merge schemas: keep old tables, add/replace with new
	schemaMap := make(map[string]manifest.Schema)
	for _, s := range old.Schemas {
		schemaMap[s.Table] = s
	}
	for _, s := range new.Schemas {
		schemaMap[s.Table] = s // new overwrites old if same table
	}
	for _, s := range old.Schemas {
		if ms, ok := schemaMap[s.Table]; ok {
			merged.Schemas = append(merged.Schemas, ms)
			delete(schemaMap, s.Table)
		}
	}
	// Add any tables only in new
	for _, s := range new.Schemas {
		if _, ok := schemaMap[s.Table]; ok {
			merged.Schemas = append(merged.Schemas, s)
		}
	}

	// Merge routes: keep old routes, add/replace with new
	type routeKey struct{ Method, Path string }
	routeMap := make(map[routeKey]manifest.Route)
	for _, r := range old.Routes {
		routeMap[routeKey{r.Method, r.Path}] = r
	}
	for _, r := range new.Routes {
		routeMap[routeKey{r.Method, r.Path}] = r
	}
	// Preserve order: old first, then new-only
	seen := make(map[routeKey]bool)
	for _, r := range old.Routes {
		k := routeKey{r.Method, r.Path}
		merged.Routes = append(merged.Routes, routeMap[k])
		seen[k] = true
	}
	for _, r := range new.Routes {
		k := routeKey{r.Method, r.Path}
		if !seen[k] {
			merged.Routes = append(merged.Routes, r)
		}
	}

	// Merge scripts: keep old, add/replace with new
	scriptMap := make(map[string]manifest.Script)
	for _, s := range old.Scripts {
		scriptMap[s.Name] = s
	}
	for _, s := range new.Scripts {
		scriptMap[s.Name] = s
	}
	seen2 := make(map[string]bool)
	for _, s := range old.Scripts {
		merged.Scripts = append(merged.Scripts, scriptMap[s.Name])
		seen2[s.Name] = true
	}
	for _, s := range new.Scripts {
		if !seen2[s.Name] {
			merged.Scripts = append(merged.Scripts, s)
		}
	}

	// Merge seeds: keep old, add new tables
	seedMap := make(map[string]manifest.Seed)
	for _, s := range old.Seeds {
		seedMap[s.Table] = s
	}
	for _, s := range new.Seeds {
		seedMap[s.Table] = s
	}
	seen3 := make(map[string]bool)
	for _, s := range old.Seeds {
		merged.Seeds = append(merged.Seeds, seedMap[s.Table])
		seen3[s.Table] = true
	}
	for _, s := range new.Seeds {
		if !seen3[s.Table] {
			merged.Seeds = append(merged.Seeds, s)
		}
	}

	return merged
}

// FormatBlueprintSummary creates a TUI-friendly summary of a blueprint.
func FormatBlueprintSummary(bp *BlueprintInfo) string {
	var b strings.Builder

	// Show steps if available
	if len(bp.Steps) > 0 {
		b.WriteString(fmt.Sprintf("Blueprint plan: %d steps", len(bp.Steps)))
		if bp.Heuristics.Score > 0 {
			b.WriteString(fmt.Sprintf(" (score: %d/10)", bp.Heuristics.Score))
		}
		b.WriteString("\n\n")
		for i, step := range bp.Steps {
			b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step))
		}
	} else if bp.Heuristics.Score > 0 {
		b.WriteString(fmt.Sprintf("Blueprint ready (score: %d/10)\n", bp.Heuristics.Score))
	} else {
		b.WriteString("Blueprint ready\n")
	}

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
