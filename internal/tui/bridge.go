package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/snapshot"
)

// Bridge subscribes to Engine Bus events and forwards them into a Bubble Tea
// program via program.Send(). This bridges the goroutine-based Bus with the
// sequential Bubble Tea event loop.
type Bridge struct {
	program *tea.Program
}

// NewBridge creates a Bridge and subscribes to all relevant Bus event types.
// Must be called after the tea.Program is created.
func NewBridge(program *tea.Program, bus *engine.Bus) *Bridge {
	b := &Bridge{program: program}

	bus.Subscribe(engine.EventRouteAdded, func(e engine.Event) {
		if detail, ok := e.Data.(string); ok {
			program.Send(RouteAddedMsg(detail))
		}
	})

	bus.Subscribe(engine.EventRouteUpdated, func(e engine.Event) {
		if detail, ok := e.Data.(string); ok {
			program.Send(RouteUpdatedMsg(detail))
		}
	})

	bus.Subscribe(engine.EventRouteRemoved, func(e engine.Event) {
		if detail, ok := e.Data.(string); ok {
			program.Send(RouteRemovedMsg(detail))
		}
	})

	bus.Subscribe(engine.EventSchemaAltered, func(e engine.Event) {
		if detail, ok := e.Data.(string); ok {
			program.Send(SchemaAlteredMsg(detail))
		}
	})

	bus.Subscribe(engine.EventDataSeeded, func(e engine.Event) {
		if detail, ok := e.Data.(string); ok {
			program.Send(DataSeededMsg(detail))
		}
	})

	bus.Subscribe(engine.EventHTTPRequestReceived, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			program.Send(HTTPRequestMsg{
				Method: data["method"],
				Path:   data["path"],
			})
		}
	})

	bus.Subscribe(engine.EventHTTPResponseSent, func(e engine.Event) {
		if data, ok := e.Data.(map[string]any); ok {
			method, _ := data["method"].(string)
			path, _ := data["path"].(string)
			status := 0
			if s, ok := data["status"].(int); ok {
				status = s
			}
			program.Send(HTTPResponseMsg{
				Method:     method,
				Path:       path,
				StatusCode: status,
			})
		}
	})

	bus.Subscribe(engine.EventSnapshotCreated, func(e engine.Event) {
		if snap, ok := e.Data.(*snapshot.Snapshot); ok {
			program.Send(SnapshotCreatedMsg{
				ID:          snap.ID,
				Description: snap.Description,
			})
		}
	})

	bus.Subscribe(engine.EventSnapshotRestored, func(e engine.Event) {
		if snap, ok := e.Data.(*snapshot.Snapshot); ok {
			program.Send(SnapshotRestoredMsg{
				ID:          snap.ID,
				Description: snap.Description,
			})
		}
	})

	bus.Subscribe(engine.EventLLMRequestStarted, func(e engine.Event) {
		if detail, ok := e.Data.(string); ok {
			program.Send(LLMStartedMsg(detail))
		}
	})

	bus.Subscribe(engine.EventLLMRequestCompleted, func(e engine.Event) {
		program.Send(LLMCompletedMsg{})
	})

	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			program.Send(LogMsg{
				Level:   data["level"],
				Message: data["message"],
			})
		}
	})

	return b
}
