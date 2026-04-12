package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/snapshot"
)

// Engine coordinates the full cycle: prompt → LLM → validate → diff → migrate → update routes.
type Engine struct {
	mu               sync.RWMutex // protects manifest and scripts
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
	ChatResponse string          // non-empty when LLM responded conversationally (no manifest changes)
	PendingSeeds []manifest.Seed // seeds awaiting user confirmation
}

// Manifest returns the current manifest.
func (e *Engine) Manifest() *manifest.Manifest {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.manifest
}

// GetScript returns the code for a named script, safe for concurrent access.
func (e *Engine) GetScript(name string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	code, ok := e.scripts[name]
	return code, ok
}

// History returns the conversation history.
func (e *Engine) History() []llm.Message {
	return e.history
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

	// 3. Plan created — generate a lightweight schema preview for the ER diagram.
	// We only need the schemas (not routes/scripts/seeds) so this is a fast, small call.
	log.Printf("[engine] plan created: %d steps — generating schema preview", len(steps))
	e.bus.Publish(Event{Type: EventPlanCreated, Data: PlanInfo{Steps: steps, Total: len(steps)}})

	schemaPrompt := fmt.Sprintf(`Based on this plan, output ONLY the "schemas" array — just the table definitions with columns, types, foreign keys. No routes, no scripts, no seeds. Output valid JSON: {"version":"1.0","name":"api","schemas":[...]}

Plan:
%s

Original request: %s`, strings.Join(steps, "\n"), prompt)

	e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: "Generating schema preview..."})
	previewManifest, genErr := e.provider.Generate(ctx, e.manifest, schemaPrompt, nil)

	var diagram string
	if genErr != nil {
		log.Printf("[engine] schema preview failed: %v — showing plan without diagram", genErr)
	} else if previewManifest != nil && len(previewManifest.Schemas) > 0 {
		diagram = manifest.GenerateMermaidER(previewManifest.Schemas)
		log.Printf("[engine] schema preview: %d tables for ER diagram", len(previewManifest.Schemas))
	}

	bp := &BlueprintInfo{
		Steps:   steps,
		Prompt:  prompt,
		Summary: fmt.Sprintf("Plan: %d steps to execute", len(steps)),
		Diagram: diagram,
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

	// 3. Validate — try autofix for compilation errors
	if err := manifest.Validate(newManifest); err != nil {
		if isCompilationError(err) {
			log.Printf("[engine] compilation error, attempting auto-fix: %v", err)
			compErrors := manifest.ValidateCompilationErrors(newManifest)
			fixed := e.autoFixManifest(ctx, newManifest, compErrors, 0, prompt)
			if fixed != nil {
				if err2 := manifest.Validate(fixed); err2 == nil {
					log.Printf("[engine] auto-fix succeeded")
					newManifest = fixed
				} else {
					log.Printf("[engine] auto-fix did not resolve all errors: %v", err2)
					e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
					return nil, fmt.Errorf("manifest validation failed: %w", err)
				}
			} else {
				e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
				return nil, fmt.Errorf("manifest validation failed: %w", err)
			}
		} else {
			log.Printf("[engine] validation failed: %v", err)
			e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
			return nil, fmt.Errorf("manifest validation failed: %w", err)
		}
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
			e.mu.Lock()
			e.scripts[c.Script.Name] = c.Script.Code
			e.mu.Unlock()
			e.bus.Publish(Event{Type: EventScriptLoaded, Data: c.Script.Name})

		case manifest.ChangeRemoveScript:
			e.mu.Lock()
			delete(e.scripts, c.Script.Name)
			e.mu.Unlock()
		}
	}

	// 8. Collect seed changes (applied separately after user confirmation).
	for _, c := range changes {
		if c.Type == manifest.ChangeAddSeed {
			result.PendingSeeds = append(result.PendingSeeds, *c.Seed)
		}
	}

	// 9. Save manifest to disk
	e.mu.Lock()
	e.manifest = newManifest
	e.mu.Unlock()
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

	// Reload the previous manifest if available
	manifestPath := filepath.Join(e.vibeDir, "manifest.json")
	if prev, err := manifest.LoadFromFile(manifestPath); err == nil {
		// Remove the last change from history
		if len(e.history) >= 2 {
			e.history = e.history[:len(e.history)-2]
		}
		e.mu.Lock()
		e.manifest = prev
		e.mu.Unlock()
	}

	return nil
}

// saveManifest writes the current manifest to .vibe/manifest.json.
func (e *Engine) saveManifest() error {
	if e.vibeDir == "" {
		return nil
	}
	e.mu.RLock()
	data, err := json.MarshalIndent(e.manifest, "", "  ")
	e.mu.RUnlock()
	if err != nil {
		return err
	}
	path := filepath.Join(e.vibeDir, "manifest.json")
	return os.WriteFile(path, data, 0o644)
}

// ApplyManifestDirect applies a manifest directly, bypassing proposal mode.
// Used by the proxy engine for auto-generated tables/routes.
func (e *Engine) ApplyManifestDirect(ctx context.Context, prompt string, m *manifest.Manifest) (*ApplyResult, error) {
	result := &ApplyResult{}
	return e.applyManifest(ctx, prompt, m, result)
}

// ApplyAutoApprove runs Apply() and immediately approves the blueprint.
// Used by the proxy engine for AI-generated responses that don't need user review.
func (e *Engine) ApplyAutoApprove(ctx context.Context, prompt string) (*ApplyResult, error) {
	blueprintResult, err := e.Apply(ctx, prompt)
	if err != nil {
		return nil, err
	}
	if blueprintResult.ChatResponse != "" {
		return &ApplyResult{ChatResponse: blueprintResult.ChatResponse}, nil
	}
	if blueprintResult.Blueprint != nil {
		return e.ApproveBlueprint(ctx)
	}
	return nil, fmt.Errorf("unexpected empty result")
}

// ApplySeeds inserts seed data into the database. Each table is seeded
// independently; failures are logged and skipped so one bad seed does not
// block the rest.
func (e *Engine) ApplySeeds(seeds []manifest.Seed) error {
	for _, seed := range seeds {
		if err := e.store.Seed(seed.Table, seed.Rows); err != nil {
			log.Printf("[engine] seed %s: %v (skipping)", seed.Table, err)
			continue
		}
		if e.bus != nil {
			e.bus.Publish(Event{Type: EventDataSeeded, Data: seed.Table})
		}
	}
	return nil
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
