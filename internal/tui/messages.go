package tui

import "github.com/vibeserve/vibeserve/internal/engine"

// Role identifies who sent a conversation message.
type Role int

const (
	RoleUser Role = iota
	RoleAssistant
	RoleSystem
	RoleError
)

// Message is a single entry in the conversation history.
type Message struct {
	Role    Role
	Content string
}

// SubmitPromptMsg is sent when the user submits a prompt.
type SubmitPromptMsg string

// ApplyResultMsg is sent when engine.Apply completes.
type ApplyResultMsg struct {
	Result *engine.ApplyResult
	Err    error
}

// UndoRequestMsg is sent when the user types /undo.
type UndoRequestMsg struct{}

// RoutesRequestMsg is sent when the user types /routes.
type RoutesRequestMsg struct{}

// StatusRequestMsg is sent when the user types /status.
type StatusRequestMsg struct{}

// UndoResultMsg is sent when engine.Undo completes.
type UndoResultMsg struct {
	Err error
}

// HTTPRequestMsg is sent when an HTTP request is received by the server.
type HTTPRequestMsg struct {
	Method string
	Path   string
}

// HTTPResponseMsg is sent when an HTTP response is sent by the server.
type HTTPResponseMsg struct {
	Method     string
	Path       string
	StatusCode int
}

// SnapshotCreatedMsg is sent when a new snapshot is created.
type SnapshotCreatedMsg struct {
	ID          int
	Description string
}

// SnapshotRestoredMsg is sent when a snapshot is restored.
type SnapshotRestoredMsg struct {
	ID          int
	Description string
}

// FlashClearMsg is sent to clear flash highlights on dashboard items.
type FlashClearMsg struct{}

// --- Engine Bus event messages ---

// RouteAddedMsg is published when a new route is registered.
type RouteAddedMsg string

// RouteUpdatedMsg is published when an existing route is modified.
type RouteUpdatedMsg string

// RouteRemovedMsg is published when a route is removed.
type RouteRemovedMsg string

// SchemaAlteredMsg is published when a schema migration completes.
type SchemaAlteredMsg string

// DataSeededMsg is published when seed data is inserted.
type DataSeededMsg string

// LLMStartedMsg is published when an LLM request begins.
type LLMStartedMsg string

// LLMCompletedMsg is published when an LLM request completes.
type LLMCompletedMsg struct{}

// StreamingChunkMsg carries incremental LLM output for live display.
type StreamingChunkMsg struct {
	Text string // accumulated text so far
}

// PlanCreatedMsg is sent when the engine creates an execution plan.
type PlanCreatedMsg struct {
	Steps []string
}

// StepProgressMsg is sent when a plan step starts or completes.
type StepProgressMsg struct {
	Index       int
	Total       int
	Description string
	Done        bool
	Summary     string // what changed (only when Done=true)
}

// LogMsg carries a log event from the engine.
type LogMsg struct {
	Level   string
	Message string
}

// BlueprintProposedMsg is sent when the engine proposes a blueprint for review.
type BlueprintProposedMsg struct {
	Blueprint *engine.BlueprintInfo
}

// BlueprintApproveMsg is sent when the user approves the pending blueprint.
type BlueprintApproveMsg struct{}

// BlueprintRefineMsg is sent when the user wants to refine the blueprint.
type BlueprintRefineMsg struct {
	Feedback string
}

// BlueprintCancelMsg is sent when the user cancels the pending blueprint.
type BlueprintCancelMsg struct{}

// BlueprintAppliedMsg is sent when the approved blueprint has been applied.
type BlueprintAppliedMsg struct {
	Result *engine.ApplyResult
	Err    error
}
