package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRootModel_Init(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")
	cmd := m.Init()
	// Init returns a batch command (non-nil)
	if cmd == nil {
		t.Error("expected Init to return a command")
	}
}

func TestRootModel_WindowResize(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updated, _ := m.Update(msg)
	root := updated.(RootModel)

	if root.width != 120 {
		t.Errorf("expected width 120, got %d", root.width)
	}
	if root.height != 40 {
		t.Errorf("expected height 40, got %d", root.height)
	}
	if !root.ready {
		t.Error("expected ready=true after WindowSizeMsg")
	}
}

func TestRootModel_WindowResize_Collapsed(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")

	// Narrow terminal: width < 100 should collapse dashboard
	msg := tea.WindowSizeMsg{Width: 80, Height: 30}
	updated, _ := m.Update(msg)
	root := updated.(RootModel)

	if root.width != 80 {
		t.Errorf("expected width 80, got %d", root.width)
	}
	// Conversation should get full width
	if root.conversation.width != 80 {
		t.Errorf("expected conversation width 80, got %d", root.conversation.width)
	}
}

func TestRootModel_TabSwitchesFocus(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")
	// Initialize with a size so it's ready
	m.width = 120
	m.height = 40
	m.ready = true

	if m.focus != PaneConversation {
		t.Error("expected initial focus on conversation")
	}

	msg := tea.KeyPressMsg{Code: tea.KeyTab}
	updated, _ := m.Update(msg)
	root := updated.(RootModel)

	if root.focus != PaneDashboard {
		t.Error("expected focus to switch to dashboard after Tab")
	}
	if root.conversation.focused {
		t.Error("expected conversation.focused=false")
	}
	if !root.dashboard.focused {
		t.Error("expected dashboard.focused=true")
	}

	// Tab again to go back
	updated, _ = root.Update(msg)
	root = updated.(RootModel)

	if root.focus != PaneConversation {
		t.Error("expected focus back on conversation after second Tab")
	}
}

func TestRootModel_CtrlC_Quits(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")
	m.width = 120
	m.height = 40
	m.ready = true

	msg := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	updated, cmd := m.Update(msg)
	root := updated.(RootModel)

	if !root.quitting {
		t.Error("expected quitting=true after Ctrl+C")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestRootModel_View_NotReady(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")
	view := m.View()
	if !strings.Contains(view.Content, "Initializing") {
		t.Error("expected 'Initializing' in view when not ready")
	}
}

func TestRootModel_View_Quitting(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")
	m.ready = true
	m.quitting = true
	view := m.View()
	if !strings.Contains(view.Content, "Shutting down") {
		t.Error("expected 'Shutting down' in view when quitting")
	}
}

func TestRootModel_ApplyResultMsg_Error(t *testing.T) {
	m := NewRootModel(nil, nil, "http://localhost:8080")
	m.width = 120
	m.height = 40
	m.ready = true

	// Add a "Thinking..." message first
	m.conversation.AddMessage(Message{Role: RoleSystem, Content: "Thinking..."})

	applyMsg := ApplyResultMsg{
		Result: nil,
		Err:    fmt.Errorf("LLM timeout"),
	}
	updated, _ := m.Update(applyMsg)
	root := updated.(RootModel)

	// Should have removed Thinking and added error
	found := false
	for _, msg := range root.conversation.messages {
		if msg.Role == RoleError && strings.Contains(msg.Content, "LLM timeout") {
			found = true
		}
	}
	if !found {
		t.Error("expected error message containing 'LLM timeout'")
	}
}
