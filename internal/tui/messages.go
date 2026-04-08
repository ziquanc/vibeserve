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
