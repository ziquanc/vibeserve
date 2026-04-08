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
	bus         *Bus
	store       SchemaStore
	trie        RouteTrie
	scripts     map[string]string
	provider    llm.Provider
	manifest    *manifest.Manifest
	history     []llm.Message
	vibeDir     string
	storeOpener func(dsn string) (SchemaStore, error)
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
	Changes  []manifest.Change
	Warnings []string
	Manifest *manifest.Manifest
}

// Manifest returns the current manifest.
func (e *Engine) Manifest() *manifest.Manifest {
	return e.manifest
}

// History returns the conversation history.
func (e *Engine) History() []llm.Message {
	return e.history
}

// Apply processes a user prompt through the full pipeline:
// 1. Emit UserPromptReceived
// 2. Call LLM to generate manifest
// 3. Validate the manifest
// 4. Diff against current manifest
// 5. Snapshot before schema changes
// 6. Apply schema migrations
// 7. Update routes and scripts
// 8. Seed new tables
// 9. Save manifest to disk
func (e *Engine) Apply(ctx context.Context, prompt string) (*ApplyResult, error) {
	result := &ApplyResult{}

	// 1. Emit UserPromptReceived
	e.bus.Publish(Event{Type: EventUserPromptReceived, Data: prompt})

	// 2. Call LLM
	e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: prompt})

	newManifest, err := e.provider.Generate(ctx, e.manifest, prompt, e.history)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	e.bus.Publish(Event{Type: EventLLMRequestCompleted, Data: newManifest})
	e.bus.Publish(Event{Type: EventManifestGenerated, Data: newManifest})

	// 3. Validate
	if err := manifest.Validate(newManifest); err != nil {
		e.bus.Publish(Event{Type: EventManifestValidationFailed, Data: err.Error()})
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	// 4. Diff
	changes := manifest.Diff(e.manifest, newManifest)
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
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeAddRoute:
			e.trie.Insert(c.Route.Method, c.Route.Path, c.Route.Script)
			e.bus.Publish(Event{Type: EventRouteAdded, Data: c.Detail})

		case manifest.ChangeUpdateRoute:
			e.trie.Remove(c.Route.Method, c.Route.Path)
			e.trie.Insert(c.Route.Method, c.Route.Path, c.Route.Script)
			e.scripts[c.Route.Script] = c.Route.Script
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

	// Update conversation history
	manifestJSON, _ := json.Marshal(newManifest)
	e.history = append(e.history,
		llm.Message{Role: "user", Content: prompt},
		llm.Message{Role: "assistant", Content: string(manifestJSON)},
	)

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
		e.manifest = prev
	}

	return nil
}

// saveManifest writes the current manifest to .vibe/manifest.json.
func (e *Engine) saveManifest() error {
	if e.vibeDir == "" {
		return nil
	}
	data, err := json.MarshalIndent(e.manifest, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(e.vibeDir, "manifest.json")
	return os.WriteFile(path, data, 0o644)
}

// FormatChangeSummary produces a human-readable summary of changes.
func FormatChangeSummary(result *ApplyResult) string {
	if len(result.Changes) == 0 {
		return "No changes detected."
	}

	var b strings.Builder
	b.WriteString("Changes applied:\n")

	for _, c := range result.Changes {
		switch c.Type {
		case manifest.ChangeAddTable:
			b.WriteString(fmt.Sprintf("  + Table: %s\n", c.Table))
		case manifest.ChangeAddColumn:
			b.WriteString(fmt.Sprintf("  + Column: %s.%s (%s)\n", c.Table, c.Column.Name, c.Column.Type))
		case manifest.ChangeDropColumn:
			b.WriteString(fmt.Sprintf("  ~ Warning: %s\n", c.Detail))
		case manifest.ChangeAddRoute:
			b.WriteString(fmt.Sprintf("  + Route: %s %s\n", c.Route.Method, c.Route.Path))
		case manifest.ChangeUpdateRoute:
			b.WriteString(fmt.Sprintf("  ~ Route: %s %s (updated)\n", c.Route.Method, c.Route.Path))
		case manifest.ChangeRemoveRoute:
			b.WriteString(fmt.Sprintf("  - Route: %s %s\n", c.Route.Method, c.Route.Path))
		case manifest.ChangeAddScript:
			b.WriteString(fmt.Sprintf("  + Script: %s\n", c.Script.Name))
		case manifest.ChangeUpdateScript:
			b.WriteString(fmt.Sprintf("  ~ Script: %s (updated)\n", c.Script.Name))
		case manifest.ChangeRemoveScript:
			b.WriteString(fmt.Sprintf("  - Script: %s\n", c.Script.Name))
		case manifest.ChangeAddSeed:
			b.WriteString(fmt.Sprintf("  + Seed: %s\n", c.Table))
		}
	}

	for _, w := range result.Warnings {
		b.WriteString(fmt.Sprintf("  ! %s\n", w))
	}

	return b.String()
}
