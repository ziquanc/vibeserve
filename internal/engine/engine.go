package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/snapshot"
)

// Engine coordinates the full cycle: prompt → LLM → validate → diff → migrate → update routes.
type Engine struct {
	bus              *Bus
	store            SchemaStore
	trie             RouteTrie
	scripts          map[string]string
	provider         llm.Provider
	manifest         *manifest.Manifest
	history          []llm.Message
	vibeDir          string
	storeOpener      func(dsn string) (SchemaStore, error)
	pendingBlueprint *BlueprintInfo
}

// EngineConfig holds configuration for creating a new Engine.
type EngineConfig struct {
	Bus      *Bus
	Store    SchemaStore
	Trie     RouteTrie
	Scripts  map[string]string
	Provider llm.Provider
	Manifest *manifest.Manifest
	VibeDir  string
	// StoreOpener is called by Undo to reopen the database after snapshot restore.
	// If nil, Undo will return an error for non-in-memory databases.
	StoreOpener func(dsn string) (SchemaStore, error)
}

// NewEngine creates a new Engine with the given configuration.
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		bus:         cfg.Bus,
		store:       cfg.Store,
		trie:        cfg.Trie,
		scripts:     cfg.Scripts,
		provider:    cfg.Provider,
		manifest:    cfg.Manifest,
		vibeDir:     cfg.VibeDir,
		storeOpener: cfg.StoreOpener,
	}
}

// ApplyResult holds the result of processing a user prompt.
type ApplyResult struct {
	Changes      []manifest.Change
	Warnings     []string
	Manifest     *manifest.Manifest
	ChatResponse string // non-empty when LLM responded conversationally (no manifest changes)
}

// Manifest returns the current manifest.
func (e *Engine) Manifest() *manifest.Manifest {
	return e.manifest
}

// History returns the conversation history.
func (e *Engine) History() []llm.Message {
	return e.history
}

// Provider returns the LLM provider.
func (e *Engine) Provider() llm.Provider {
	return e.provider
}

// Bus returns the event bus.
func (e *Engine) Bus() *Bus {
	return e.bus
}

// Scripts returns the scripts map.
func (e *Engine) Scripts() map[string]string {
	return e.scripts
}

// Store returns the schema store.
func (e *Engine) Store() SchemaStore {
	return e.store
}

// Trie returns the route trie.
func (e *Engine) Trie() RouteTrie {
	return e.trie
}

// SetScripts replaces the scripts map (used by proxy to add new scripts).
func (e *Engine) SetScripts(s map[string]string) {
	e.scripts = s
}

// Apply processes a user prompt through the full pipeline and PROPOSES a blueprint:
// 1. Emit UserPromptReceived
// 2. Call LLM to generate manifest (with planning if needed)
// 3. Propose the resulting manifest (validate, diff, score, store as pending)
// 4. Emit EventBlueprintProposed — actual application happens via ApproveBlueprint()
func (e *Engine) Apply(ctx context.Context, prompt string) (*BlueprintResult, error) {
	// 1. Emit UserPromptReceived
	e.bus.Publish(Event{Type: EventUserPromptReceived, Data: prompt})

	// 2. Planning phase — ask LLM to break work into steps
	e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: "Planning..."})

	planPrompt := llm.BuildPlanPrompt(prompt)
	planManifest, planErr := e.provider.Generate(ctx, e.manifest, planPrompt, nil)

	var steps []string
	if planErr != nil {
		if chatErr, ok := planErr.(*llm.ChatOnlyError); ok {
			// Try to parse as plan (JSON array)
			parsed, parseErr := llm.ExtractPlan(chatErr.Text)
			if parseErr == nil && len(parsed) > 0 {
				steps = parsed
			} else {
				// Not a plan — might be a conversational response to a question
				e.history = append(e.history, llm.Message{Role: "user", Content: prompt})
				e.history = append(e.history, llm.Message{Role: "assistant", Content: chatErr.Text})
				return &BlueprintResult{ChatResponse: chatErr.Text}, nil
			}
		} else {
			return nil, fmt.Errorf("planning failed: %w", planErr)
		}
	}

	// If LLM returned a manifest directly (simple request), propose it directly
	if planManifest != nil && len(steps) == 0 {
		log.Printf("[engine] LLM returned manifest directly (no planning needed)")
		bp, err := e.proposeBlueprint(planManifest)
		if err != nil {
			return nil, err
		}
		e.bus.Publish(Event{Type: EventBlueprintProposed, Data: *bp})
		return &BlueprintResult{Blueprint: bp}, nil
	}

	// If no steps parsed, fall back to single-step direct generation
	if len(steps) == 0 {
		log.Printf("[engine] no plan created, falling back to direct generation")
		return e.directApply(ctx, prompt)
	}

	// 3. Propose the plan as a blueprint — steps are NOT executed yet.
	// Execution happens in ApproveBlueprint() after the user confirms.
	log.Printf("[engine] plan created: %d steps — proposing for review", len(steps))
	e.bus.Publish(Event{Type: EventPlanCreated, Data: PlanInfo{Steps: steps, Total: len(steps)}})

	bp := &BlueprintInfo{
		Steps:   steps,
		Prompt:  prompt,
		Summary: fmt.Sprintf("Plan: %d steps to execute", len(steps)),
	}
	e.pendingBlueprint = bp
	e.bus.Publish(Event{Type: EventBlueprintProposed, Data: *bp})
	return &BlueprintResult{Blueprint: bp}, nil
}

// directApply does a single LLM call without planning (fallback) and proposes the result.
func (e *Engine) directApply(ctx context.Context, prompt string) (*BlueprintResult, error) {
	e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: prompt})

	newManifest, err := e.provider.Generate(ctx, e.manifest, prompt, e.history)
	if err != nil {
		if chatErr, ok := err.(*llm.ChatOnlyError); ok {
			e.history = append(e.history, llm.Message{Role: "user", Content: prompt})
			e.history = append(e.history, llm.Message{Role: "assistant", Content: chatErr.Text})
			return &BlueprintResult{ChatResponse: chatErr.Text}, nil
		}
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	bp, err := e.proposeBlueprint(newManifest)
	if err != nil {
		return nil, err
	}
	e.bus.Publish(Event{Type: EventBlueprintProposed, Data: *bp})
	return &BlueprintResult{Blueprint: bp}, nil
}

// applyManifest validates, diffs, and applies a new manifest.
func (e *Engine) applyManifest(ctx context.Context, prompt string, newManifest *manifest.Manifest, result *ApplyResult) (*ApplyResult, error) {
	e.bus.Publish(Event{Type: EventLLMRequestCompleted, Data: newManifest})

	// 2.5. Auto-repair common LLM omissions
	repairManifest(newManifest, e.manifest)

	log.Printf("[engine] manifest received: %s (%d schemas, %d routes, %d scripts)",
		newManifest.Name, len(newManifest.Schemas), len(newManifest.Routes), len(newManifest.Scripts))

	e.bus.Publish(Event{Type: EventManifestGenerated, Data: newManifest})

	// 3. Validate
	if err := manifest.Validate(newManifest); err != nil {
		log.Printf("[engine] validation failed: %v", err)
		e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}
	log.Printf("[engine] manifest validated OK")

	// 4. Diff
	changes := manifest.Diff(e.manifest, newManifest)
	log.Printf("[engine] diff computed: %d changes", len(changes))
	e.bus.Publish(Event{Type: EventManifestDiffComputed, Data: changes})
	result.Changes = changes

	// 5 & 6. Apply schema changes
	hasSchemaChanges := false
	for _, c := range changes {
		if c.Type == manifest.ChangeAddTable || c.Type == manifest.ChangeAddColumn {
			hasSchemaChanges = true
			break
		}
	}

	if hasSchemaChanges {
		// Snapshot before schema changes
		dbPath := e.store.DSN()
		if dbPath != ":memory:" && dbPath != "" {
			e.bus.Publish(Event{Type: EventSchemaAltering, Data: "creating snapshot"})
			snap, snapErr := snapshot.Create(e.vibeDir, dbPath, "before_schema_change")
			if snapErr != nil {
				log.Printf("Warning: failed to create snapshot: %v", snapErr)
			} else {
				e.bus.Publish(Event{Type: EventSnapshotCreated, Data: snap})
			}
		}

		for _, c := range changes {
			switch c.Type {
			case manifest.ChangeAddTable:
				e.bus.Publish(Event{Type: EventSchemaAltering, Data: c.Detail})
				if err := e.store.ApplySchemas([]manifest.Schema{*c.Schema}); err != nil {
					e.bus.Publish(Event{Type: EventSchemaMigrationFailed, Data: err.Error()})
					return nil, fmt.Errorf("schema migration failed: %w", err)
				}
				e.bus.Publish(Event{Type: EventSchemaAltered, Data: c.Detail})

			case manifest.ChangeAddColumn:
				e.bus.Publish(Event{Type: EventSchemaAltering, Data: c.Detail})
				if err := e.store.AddColumn(c.Table, *c.Column); err != nil {
					e.bus.Publish(Event{Type: EventSchemaMigrationFailed, Data: err.Error()})
					return nil, fmt.Errorf("add column failed: %w", err)
				}
				e.bus.Publish(Event{Type: EventSchemaAltered, Data: c.Detail})

			case manifest.ChangeDropColumn:
				result.Warnings = append(result.Warnings, c.Detail)
			}
		}
	}

	// Check for drop column warnings even if no adds
	if !hasSchemaChanges {
		for _, c := range changes {
			if c.Type == manifest.ChangeDropColumn {
				result.Warnings = append(result.Warnings, c.Detail)
			}
		}
	}

	// 7. Update routes and scripts
	log.Printf("[engine] applying %d changes to routes and scripts", len(changes))
	for i, c := range changes {
		log.Printf("[engine] change %d/%d: %s %s", i+1, len(changes), c.Type, c.Detail)
		switch c.Type {
		case manifest.ChangeAddRoute:
			e.trie.Insert(c.Route.Method, c.Route.Path, c.Route.Script)
			e.bus.Publish(Event{Type: EventRouteAdded, Data: c.Detail})

		case manifest.ChangeUpdateRoute:
			e.trie.Remove(c.Route.Method, c.Route.Path)
			e.trie.Insert(c.Route.Method, c.Route.Path, c.Route.Script)
			e.bus.Publish(Event{Type: EventRouteUpdated, Data: c.Detail})

		case manifest.ChangeRemoveRoute:
			e.trie.Remove(c.Route.Method, c.Route.Path)
			e.bus.Publish(Event{Type: EventRouteRemoved, Data: c.Detail})

		case manifest.ChangeAddScript, manifest.ChangeUpdateScript:
			e.bus.Publish(Event{Type: EventScriptValidationStarted, Data: c.Script.Name})
			e.scripts[c.Script.Name] = c.Script.Code
			e.bus.Publish(Event{Type: EventScriptLoaded, Data: c.Script.Name})

		case manifest.ChangeRemoveScript:
			delete(e.scripts, c.Script.Name)
		}
	}

	// 8. Seed new tables
	for _, c := range changes {
		if c.Type == manifest.ChangeAddSeed {
			if err := e.store.Seed(c.Seed.Table, c.Seed.Rows); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("seed %s failed: %v", c.Seed.Table, err))
			} else {
				e.bus.Publish(Event{Type: EventDataSeeded, Data: c.Seed.Table})
			}
		}
	}

	// 9. Save manifest to disk
	e.manifest = newManifest
	result.Manifest = newManifest

	if err := e.saveManifest(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("failed to save manifest: %v", err))
	}
	log.Printf("[engine] manifest saved, %d total routes now active", len(newManifest.Routes))

	// Update conversation history
	manifestJSON, _ := json.Marshal(newManifest)
	e.history = append(e.history,
		llm.Message{Role: "user", Content: prompt},
		llm.Message{Role: "assistant", Content: string(manifestJSON)},
	)

	log.Printf("[engine] Apply complete: %d changes, %d warnings", len(result.Changes), len(result.Warnings))
	return result, nil
}

// Undo restores the most recent snapshot and reloads the previous manifest.
func (e *Engine) Undo() error {
	dbPath := e.store.DSN()
	if dbPath == ":memory:" || dbPath == "" {
		return fmt.Errorf("undo not supported with in-memory database")
	}

	// Close the current store
	e.store.Close()

	// Restore the latest snapshot
	snap, err := snapshot.RestoreLatest(e.vibeDir, dbPath)
	if err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}

	// Reopen the store
	if e.storeOpener == nil {
		return fmt.Errorf("undo: no StoreOpener configured")
	}
	newStore, err := e.storeOpener(dbPath)
	if err != nil {
		return fmt.Errorf("reopen store: %w", err)
	}
	e.store = newStore

	e.bus.Publish(Event{Type: EventSnapshotRestored, Data: snap})

	// Restore the previous manifest: copy manifest.prev.json over manifest.json,
	// then load it.
	prevPath := filepath.Join(e.vibeDir, "manifest.prev.json")
	manifestPath := filepath.Join(e.vibeDir, "manifest.json")
	if prevData, err := os.ReadFile(prevPath); err == nil {
		_ = os.WriteFile(manifestPath, prevData, 0o644)
	}

	if prev, err := manifest.LoadFromFile(manifestPath); err == nil {
		// Remove the last change from history
		if len(e.history) >= 2 {
			e.history = e.history[:len(e.history)-2]
		}
		e.manifest = prev
	}

	return nil
}

// saveManifest writes the current manifest to .vibe/manifest.json.
// It first backs up the existing manifest.json to manifest.prev.json so that
// Undo() can restore both the DB snapshot and the previous manifest.
func (e *Engine) saveManifest() error {
	if e.vibeDir == "" {
		return nil
	}
	manifestPath := filepath.Join(e.vibeDir, "manifest.json")
	prevPath := filepath.Join(e.vibeDir, "manifest.prev.json")

	// Back up the current manifest before overwriting.
	if existing, err := os.ReadFile(manifestPath); err == nil {
		_ = os.WriteFile(prevPath, existing, 0o644)
	}

	data, err := json.MarshalIndent(e.manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, data, 0o644)
}

// repairManifest fills in common fields that LLMs often omit.
// This runs before validation to avoid rejecting otherwise-good output.
func repairManifest(m *manifest.Manifest, previous *manifest.Manifest) {
	if m.Version == "" {
		m.Version = "1.0"
	}
	if m.Name == "" {
		if previous != nil && previous.Name != "" {
			m.Name = previous.Name
		} else if m.Description != "" {
			// Derive name from description
			name := strings.ToLower(m.Description)
			name = strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
					return r
				}
				if r == ' ' {
					return '-'
				}
				return -1
			}, name)
			if len(name) > 40 {
				name = name[:40]
			}
			if name == "" {
				name = "my-api"
			}
			m.Name = name
		} else {
			m.Name = "my-api"
		}
	}
	if m.Schemas == nil {
		m.Schemas = []manifest.Schema{}
	}
	if m.Routes == nil {
		m.Routes = []manifest.Route{}
	}
	if m.Scripts == nil {
		m.Scripts = []manifest.Script{}
	}
	if m.Seeds == nil {
		m.Seeds = []manifest.Seed{}
	}
}

// FormatChangeSummary produces a clear, readable summary of what changed.
func FormatChangeSummary(result *ApplyResult) string {
	if result.ChatResponse != "" {
		return result.ChatResponse
	}
	if len(result.Changes) == 0 {
		return "No changes detected."
	}

	var b strings.Builder

	// Group changes by type for clearer output
	var tables, columns, routes, scripts, seeds []string
	var warnings []string

	for _, c := range result.Changes {
		switch c.Type {
		case manifest.ChangeAddTable:
			cols := ""
			if c.Schema != nil {
				cols = fmt.Sprintf(" (%d columns)", len(c.Schema.Columns))
			}
			tables = append(tables, fmt.Sprintf("  + Created table: %s%s", c.Table, cols))
		case manifest.ChangeAddColumn:
			columns = append(columns, fmt.Sprintf("  + Added column: %s.%s (%s)", c.Table, c.Column.Name, c.Column.Type))
		case manifest.ChangeDropColumn:
			warnings = append(warnings, fmt.Sprintf("  ! Column %s.%s was removed from manifest (not applied to DB)", c.Table, c.Column.Name))
		case manifest.ChangeAddRoute:
			routes = append(routes, fmt.Sprintf("  + Added route: %s %s", c.Route.Method, c.Route.Path))
		case manifest.ChangeUpdateRoute:
			routes = append(routes, fmt.Sprintf("  ~ Updated route: %s %s", c.Route.Method, c.Route.Path))
		case manifest.ChangeRemoveRoute:
			routes = append(routes, fmt.Sprintf("  - Removed route: %s %s", c.Route.Method, c.Route.Path))
		case manifest.ChangeAddScript:
			scripts = append(scripts, fmt.Sprintf("  + Added script: %s", c.Script.Name))
		case manifest.ChangeUpdateScript:
			scripts = append(scripts, fmt.Sprintf("  ~ Updated script: %s", c.Script.Name))
		case manifest.ChangeRemoveScript:
			scripts = append(scripts, fmt.Sprintf("  - Removed script: %s", c.Script.Name))
		case manifest.ChangeAddSeed:
			count := 0
			if c.Seed != nil {
				count = len(c.Seed.Rows)
			}
			seeds = append(seeds, fmt.Sprintf("  + Seeded table: %s (%d rows)", c.Table, count))
		}
	}

	// Print grouped sections
	if len(tables) > 0 || len(columns) > 0 {
		b.WriteString("Schema:\n")
		for _, s := range tables {
			b.WriteString(s + "\n")
		}
		for _, s := range columns {
			b.WriteString(s + "\n")
		}
		b.WriteString("\n")
	}

	if len(routes) > 0 {
		b.WriteString("Routes:\n")
		for _, s := range routes {
			b.WriteString(s + "\n")
		}
		b.WriteString("\n")
	}

	if len(scripts) > 0 {
		b.WriteString("Scripts:\n")
		for _, s := range scripts {
			b.WriteString(s + "\n")
		}
		b.WriteString("\n")
	}

	if len(seeds) > 0 {
		b.WriteString("Data:\n")
		for _, s := range seeds {
			b.WriteString(s + "\n")
		}
		b.WriteString("\n")
	}

	if len(warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, s := range warnings {
			b.WriteString(s + "\n")
		}
		b.WriteString("\n")
	}

	for _, w := range result.Warnings {
		b.WriteString(fmt.Sprintf("  ! %s\n", w))
	}

	// Show a test hint
	if len(routes) > 0 && result.Manifest != nil {
		b.WriteString("Try it:\n")
		for _, r := range result.Manifest.Routes {
			if r.Method == "GET" {
				b.WriteString(fmt.Sprintf("  curl http://localhost:8080%s\n", r.Path))
				break
			}
		}
	}

	return b.String()
}

// ApplyAutoApprove runs the full Apply pipeline but auto-approves the blueprint
// without requiring user interaction. This is used by the proxy/auto-evolve mode.
// It returns the ApplyResult from applying the generated manifest.
func (e *Engine) ApplyAutoApprove(ctx context.Context, prompt string) (*ApplyResult, error) {
	// Clear any pending blueprint first
	e.pendingBlueprint = nil

	// Direct LLM generation without planning (for speed in proxy mode)
	e.bus.Publish(Event{Type: EventUserPromptReceived, Data: prompt})
	e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: prompt})

	newManifest, err := e.provider.Generate(ctx, e.manifest, prompt, e.history)
	if err != nil {
		if chatErr, ok := err.(*llm.ChatOnlyError); ok {
			return nil, fmt.Errorf("LLM returned conversational text instead of manifest: %s", chatErr.Text[:min(200, len(chatErr.Text))])
		}
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	if newManifest == nil {
		return nil, fmt.Errorf("LLM returned nil manifest")
	}

	// Propose and immediately approve
	bp, err := e.proposeBlueprint(newManifest)
	if err != nil {
		return nil, fmt.Errorf("propose blueprint: %w", err)
	}

	// Clear pending so ApproveBlueprint can consume it
	_ = bp

	result, err := e.ApproveBlueprint(ctx)
	if err != nil {
		return nil, fmt.Errorf("approve blueprint: %w", err)
	}

	return result, nil
}
