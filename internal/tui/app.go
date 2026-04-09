package tui

import (
	"context"
	"fmt"
	"strings"

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

	case tea.PasteMsg:
		// Forward paste events to conversation
		var cmd tea.Cmd
		m.conversation, cmd = m.conversation.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

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

	case RoutesRequestMsg:
		if m.engine == nil || m.engine.Manifest() == nil || len(m.engine.Manifest().Routes) == 0 {
			m.conversation.AddMessage(Message{Role: RoleAssistant, Content: "No routes defined yet."})
		} else {
			var lines []string
			lines = append(lines, "API Routes:")
			lines = append(lines, "")
			for _, r := range m.engine.Manifest().Routes {
				lines = append(lines, fmt.Sprintf("  %-7s %s  →  %s", r.Method, r.Path, r.Script))
			}
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("Base URL: http://%s", m.header.serverURL))
			m.conversation.AddMessage(Message{Role: RoleAssistant, Content: strings.Join(lines, "\n")})
		}

	case StatusRequestMsg:
		if m.engine == nil || m.engine.Manifest() == nil {
			m.conversation.AddMessage(Message{Role: RoleAssistant, Content: "No project loaded. Type a prompt to create your API."})
		} else {
			man := m.engine.Manifest()
			var lines []string
			lines = append(lines, fmt.Sprintf("Project: %s", man.Name))
			if man.Description != "" {
				lines = append(lines, fmt.Sprintf("  %s", man.Description))
			}
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("  Tables:  %d", len(man.Schemas)))
			for _, s := range man.Schemas {
				lines = append(lines, fmt.Sprintf("    - %s (%d columns)", s.Table, len(s.Columns)))
			}
			lines = append(lines, fmt.Sprintf("  Routes:  %d", len(man.Routes)))
			lines = append(lines, fmt.Sprintf("  Scripts: %d", len(man.Scripts)))
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("  Server: http://%s", m.header.serverURL))
			lines = append(lines, fmt.Sprintf("  Data:   .vibe/state.db"))
			m.conversation.AddMessage(Message{Role: RoleAssistant, Content: strings.Join(lines, "\n")})
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

	case BlueprintProposedMsg:
		m.conversation.RemoveLastSystem()
		m.conversation.proposalPending = true

		summary := engine.FormatBlueprintSummary(msg.Blueprint)
		m.conversation.AddMessage(Message{
			Role:    RoleAssistant,
			Content: summary,
		})

		m.conversation.AddMessage(Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Full blueprint: %s/_blueprint", m.serverURL),
		})

		m.conversation.AddMessage(Message{
			Role:    RoleSystem,
			Content: "[y] approve  |  type feedback to refine  |  [n] cancel",
		})

	case BlueprintApproveMsg:
		m.conversation.proposalPending = false
		m.conversation.AddMessage(Message{Role: RoleSystem, Content: "Applying blueprint..."})
		cmd := m.approveBlueprint()
		cmds = append(cmds, cmd)

	case BlueprintRefineMsg:
		cmd := m.refineBlueprint(msg.Feedback)
		cmds = append(cmds, cmd)

	case BlueprintCancelMsg:
		if m.engine != nil {
			m.engine.CancelBlueprint()
		}

	case BlueprintAppliedMsg:
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
		// Update the last system message with streaming progress
		m.conversation.UpdateLastSystem(msg.Text)

	case PlanCreatedMsg:
		m.conversation.RemoveLastSystem()
		m.conversation.AddMessage(Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Plan: %d steps to execute", len(msg.Steps)),
		})
		for i, step := range msg.Steps {
			m.conversation.AddMessage(Message{
				Role:    RoleSystem,
				Content: fmt.Sprintf("  %d. %s", i+1, step),
			})
		}

	case StepProgressMsg:
		if !msg.Done {
			m.conversation.AddMessage(Message{
				Role:    RoleSystem,
				Content: fmt.Sprintf("Step %d/%d: %s...", msg.Index, msg.Total, msg.Description),
			})
		} else {
			m.conversation.AddMessage(Message{
				Role:    RoleAssistant,
				Content: fmt.Sprintf("Step %d/%d done: %s", msg.Index, msg.Total, msg.Summary),
			})
		}

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
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = ApplyResultMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		result, err := eng.Apply(ctx, prompt)
		if err != nil {
			return ApplyResultMsg{Err: err}
		}
		if result.Blueprint != nil {
			return BlueprintProposedMsg{Blueprint: result.Blueprint}
		}
		if result.ChatResponse != "" {
			return ApplyResultMsg{Result: &engine.ApplyResult{ChatResponse: result.ChatResponse}}
		}
		return ApplyResultMsg{Err: fmt.Errorf("unexpected empty result")}
	}
}

// applyUndo dispatches engine.Undo as a tea.Cmd.
func (m RootModel) applyUndo() tea.Cmd {
	eng := m.engine
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = UndoResultMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		err := eng.Undo()
		return UndoResultMsg{Err: err}
	}
}

// approveBlueprint dispatches engine.ApproveBlueprint as a tea.Cmd.
func (m RootModel) approveBlueprint() tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = BlueprintAppliedMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		result, err := eng.ApproveBlueprint(ctx)
		return BlueprintAppliedMsg{Result: result, Err: err}
	}
}

// refineBlueprint dispatches engine.RefineBlueprint as a tea.Cmd.
func (m RootModel) refineBlueprint(feedback string) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = ApplyResultMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		bp, err := eng.RefineBlueprint(ctx, feedback)
		if err != nil {
			return ApplyResultMsg{Err: err}
		}
		return BlueprintProposedMsg{Blueprint: bp}
	}
}
