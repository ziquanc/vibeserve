package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConversation_TypeCharacter(t *testing.T) {
	m := NewConversationModel()
	m.focused = true
	m.SetSize(80, 20)

	m, _ = m.Update(tea.KeyPressMsg{Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "b"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "c"})

	if m.input != "abc" {
		t.Errorf("expected input 'abc', got %q", m.input)
	}
	if m.cursorPos != 3 {
		t.Errorf("expected cursorPos 3, got %d", m.cursorPos)
	}
}

func TestConversation_Backspace(t *testing.T) {
	m := NewConversationModel()
	m.focused = true
	m.input = "hello"
	m.cursorPos = 5

	m, _ = m.Update(tea.KeyPressMsg{Text: "backspace"})

	if m.input != "hell" {
		t.Errorf("expected 'hell', got %q", m.input)
	}
	if m.cursorPos != 4 {
		t.Errorf("expected cursorPos 4, got %d", m.cursorPos)
	}
}

func TestConversation_Submit(t *testing.T) {
	m := NewConversationModel()
	m.focused = true
	m.input = "create a users API"
	m.cursorPos = len(m.input)

	_, cmd := m.Update(tea.KeyPressMsg{Text: "enter"})

	if cmd == nil {
		t.Fatal("expected a command from Enter")
	}

	msg := cmd()
	submitMsg, ok := msg.(SubmitPromptMsg)
	if !ok {
		t.Fatalf("expected SubmitPromptMsg, got %T", msg)
	}
	if string(submitMsg) != "create a users API" {
		t.Errorf("expected 'create a users API', got %q", string(submitMsg))
	}
}

func TestConversation_EmptySubmit(t *testing.T) {
	m := NewConversationModel()
	m.focused = true
	m.input = "   "

	_, cmd := m.Update(tea.KeyPressMsg{Text: "enter"})

	if cmd != nil {
		t.Error("expected nil command for empty input")
	}
}

func TestConversation_AddMessage(t *testing.T) {
	m := NewConversationModel()
	m.AddMessage(Message{Role: RoleUser, Content: "hello"})
	m.AddMessage(Message{Role: RoleAssistant, Content: "world"})

	if len(m.messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(m.messages))
	}
	if m.messages[0].Role != RoleUser {
		t.Error("expected first message to be user")
	}
	if m.messages[1].Content != "world" {
		t.Errorf("expected second message content 'world', got %q", m.messages[1].Content)
	}
}

func TestConversation_ViewShowsInput(t *testing.T) {
	m := NewConversationModel()
	m.SetSize(80, 20)
	m.focused = true

	view := m.View()
	if !strings.Contains(view, "vibe>") {
		t.Error("expected 'vibe>' prompt in view")
	}
}

func TestConversation_ViewShowsMessages(t *testing.T) {
	m := NewConversationModel()
	m.SetSize(80, 20)
	m.AddMessage(Message{Role: RoleUser, Content: "hello"})
	m.AddMessage(Message{Role: RoleAssistant, Content: "world"})

	view := m.View()
	if !strings.Contains(view, "hello") {
		t.Error("expected 'hello' in conversation view")
	}
	if !strings.Contains(view, "world") {
		t.Error("expected 'world' in conversation view")
	}
}

func TestConversation_BackspaceAtStart(t *testing.T) {
	m := NewConversationModel()
	m.input = "hi"
	m.cursorPos = 0

	// Backspace at start should be a no-op
	m, _ = m.Update(tea.KeyPressMsg{Text: "backspace"})

	if m.input != "hi" {
		t.Errorf("expected input unchanged 'hi', got %q", m.input)
	}
	if m.cursorPos != 0 {
		t.Errorf("expected cursorPos 0, got %d", m.cursorPos)
	}
}

func TestConversation_RemoveLastSystem(t *testing.T) {
	m := NewConversationModel()
	m.AddMessage(Message{Role: RoleUser, Content: "hello"})
	m.AddMessage(Message{Role: RoleSystem, Content: "Thinking..."})
	m.AddMessage(Message{Role: RoleSystem, Content: "Processing..."})

	m.RemoveLastSystem()

	if len(m.messages) != 2 {
		t.Errorf("expected 2 messages after removal, got %d", len(m.messages))
	}
	if m.messages[1].Content != "Thinking..." {
		t.Errorf("expected 'Thinking...' to remain, got %q", m.messages[1].Content)
	}
}

func TestConversation_EmptyView(t *testing.T) {
	m := NewConversationModel()
	m.SetSize(80, 20)

	view := m.View()
	if !strings.Contains(view, "VibeServe") {
		t.Error("expected welcome screen in empty conversation view")
	}
}

func TestConversation_UndoCommand(t *testing.T) {
	m := NewConversationModel()
	m.focused = true
	m.input = "/undo"
	m.cursorPos = 5

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "enter"})

	if cmd == nil {
		t.Fatal("expected a command for undo")
	}

	msg := cmd()
	if _, ok := msg.(UndoRequestMsg); !ok {
		t.Fatalf("expected UndoRequestMsg, got %T", msg)
	}

	if updated.input != "" {
		t.Errorf("expected empty input after undo, got %q", updated.input)
	}
}
