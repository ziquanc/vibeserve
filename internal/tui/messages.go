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
