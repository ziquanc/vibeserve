package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/vibeserve/vibeserve/internal/engine"
)

// Pane identifies which side of the split layout has focus.
type Pane int

const (
	PaneConversation Pane = iota
	PaneDashboard
)

// RootModel is the top-level Bubble Tea model for the VibeServe TUI.
type RootModel struct {
	// Layout
	width  int
	height int
	focus  Pane

	// Sub-models
	header       HeaderModel
	conversation ConversationModel
	dashboard    DashboardModel
	statusBar    StatusBarModel

	// Engine integration
	engine    *engine.Engine
	bus       *engine.Bus
	serverURL string
	ctx       context.Context
	cancel    context.CancelFunc

	// State
	ready    bool
	quitting bool
}

// NewRootModel creates a RootModel wired to the given engine and bus.
func NewRootModel(eng *engine.Engine, bus *engine.Bus, serverURL string) RootModel {
	ctx, cancel := context.WithCancel(context.Background())
	return RootModel{
		focus:        PaneConversation,
		header:       NewHeaderModel(serverURL),
		conversation: NewConversationModel(),
		dashboard:    NewDashboardModel(),
		statusBar:    NewStatusBarModel(),
		engine:       eng,
		bus:          bus,
		serverURL:    serverURL,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Init satisfies tea.Model. Requests window size on startup.
func (m RootModel) Init() tea.Cmd {
	requestSize := func() tea.Msg {
		return tea.RequestWindowSize()
	}
	return tea.Batch(
		requestSize,
		m.conversation.Init(),
	)
}

// Update handles all incoming messages and delegates to sub-models.
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Distribute sizes to sub-models
		m.header.width = m.width
		m.statusBar.width = m.width

		contentHeight := m.height - headerHeight - statusBarHeight
		if contentHeight < 1 {
			contentHeight = 1
		}

		// Full-width chat
		m.conversation.SetSize(m.width, contentHeight)

		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			m.cancel()
			return m, tea.Quit

		}

		// All keys go to conversation
		var cmd tea.Cmd
		m.conversation, cmd = m.conversation.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case SubmitPromptMsg:
		// User pressed Enter with a non-empty prompt
		prompt := string(msg)
		m.conversation.AddMessage(Message{Role: RoleUser, Content: prompt})
		m.conversation.AddMessage(Message{Role: RoleSystem, Content: "Thinking..."})

		cmd := m.applyPrompt(prompt)
		cmds = append(cmds, cmd)

	case ApplyResultMsg:
		// Remove the "Thinking..." message
		m.conversation.RemoveLastSystem()

		if msg.Err != nil {
			m.conversation.AddMessage(Message{
				Role:    RoleError,
				Content: fmt.Sprintf("Error: %v", msg.Err),
			})
		} else {
			summary := engine.FormatChangeSummary(msg.Result)
			m.conversation.AddMessage(Message{
				Role:    RoleAssistant,
				Content: summary,
			})
			// Update dashboard with new manifest state
			if msg.Result != nil && msg.Result.Manifest != nil {
				m.dashboard.UpdateFromManifest(msg.Result.Manifest)
			}
		}

	case UndoResultMsg:
		m.conversation.RemoveLastSystem()
		if msg.Err != nil {
			m.conversation.AddMessage(Message{
				Role:    RoleError,
				Content: fmt.Sprintf("Undo failed: %v", msg.Err),
			})
		} else {
			m.conversation.AddMessage(Message{
				Role:    RoleAssistant,
				Content: "Undo successful",
			})
		}

	case StreamingChunkMsg:
		// Update the last system message ("Thinking...") with streaming text
		m.conversation.UpdateLastSystem("Generating: " + fmt.Sprintf("%d chars received...", len(msg.Text)))

	// Engine bus events
	case RouteAddedMsg:
		m.dashboard.AddRouteEvent(string(msg))
	case RouteUpdatedMsg:
		m.dashboard.AddRouteEvent(string(msg))
	case RouteRemovedMsg:
		m.dashboard.AddRouteEvent(string(msg))
	case HTTPRequestMsg:
		m.dashboard.AddHTTPTrace(msg)
	case HTTPResponseMsg:
		m.dashboard.AddHTTPTrace(msg)
	case SnapshotCreatedMsg:
		m.dashboard.AddSnapshot(msg)
	case SnapshotRestoredMsg:
		m.dashboard.AddSnapshot(msg)
	case SchemaAlteredMsg:
		m.dashboard.AddSchemaEvent(string(msg))
	case DataSeededMsg:
		m.dashboard.AddSchemaEvent(string(msg))
	case LogMsg:
		m.conversation.AddMessage(Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("[%s] %s", msg.Level, msg.Message),
		})
	}

	return m, tea.Batch(cmds...)
}

// View renders the full TUI.
func (m RootModel) View() tea.View {
	if m.quitting {
		v := tea.NewView("Shutting down...")
		v.AltScreen = true
		return v
	}
	if !m.ready {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	header := m.header.View()
	status := m.statusBar.View()

	// Full-width chat — like Claude Code
	content := m.conversation.View()

	v := tea.NewView(header + "\n" + content + "\n" + status)
	v.AltScreen = true
	return v
}

// applyPrompt dispatches engine.Apply as a tea.Cmd (runs in a goroutine).
func (m RootModel) applyPrompt(prompt string) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() tea.Msg {
		result, err := eng.Apply(ctx, prompt)
		return ApplyResultMsg{Result: result, Err: err}
	}
}

// applyUndo dispatches engine.Undo as a tea.Cmd.
func (m RootModel) applyUndo() tea.Cmd {
	eng := m.engine
	return func() tea.Msg {
		err := eng.Undo()
		return UndoResultMsg{Err: err}
	}
}
