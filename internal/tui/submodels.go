package tui

import (
	tea "charm.land/bubbletea/v2"
)

// HeaderModel renders the top bar.
type HeaderModel struct {
	width     int
	serverURL string
}

// NewHeaderModel creates a HeaderModel.
func NewHeaderModel(serverURL string) HeaderModel {
	return HeaderModel{serverURL: serverURL}
}

// View renders the header.
func (m HeaderModel) View() string {
	return "VibeServe | " + m.serverURL
}

// ConversationModel manages the conversation pane.
type ConversationModel struct {
	width    int
	height   int
	focused  bool
	messages []Message
}

// NewConversationModel creates a ConversationModel.
func NewConversationModel() ConversationModel {
	return ConversationModel{focused: true}
}

// Init returns no command.
func (m ConversationModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the conversation pane.
func (m ConversationModel) Update(msg tea.Msg) (ConversationModel, tea.Cmd) {
	return m, nil
}

// View renders the conversation pane.
func (m ConversationModel) View() string {
	return ""
}

// SetSize sets the pane dimensions.
func (m *ConversationModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// AddMessage appends a message to the conversation.
func (m *ConversationModel) AddMessage(msg Message) {
	m.messages = append(m.messages, msg)
}

// RemoveLastSystem removes the last system message.
func (m *ConversationModel) RemoveLastSystem() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == RoleSystem {
			m.messages = append(m.messages[:i], m.messages[i+1:]...)
			return
		}
	}
}

// DashboardModel manages the dashboard pane.
type DashboardModel struct {
	width   int
	height  int
	focused bool
}

// NewDashboardModel creates a DashboardModel.
func NewDashboardModel() DashboardModel {
	return DashboardModel{}
}

// Update handles messages for the dashboard pane.
func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	return m, nil
}

// View renders the dashboard pane.
func (m DashboardModel) View() string {
	return ""
}

// SetSize sets the pane dimensions.
func (m *DashboardModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// UpdateFromManifest refreshes dashboard state from a manifest.
func (m *DashboardModel) UpdateFromManifest(manifest interface{}) {}

// StatusBarModel renders the bottom status bar.
type StatusBarModel struct {
	width int
}

// NewStatusBarModel creates a StatusBarModel.
func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{}
}

// View renders the status bar.
func (m StatusBarModel) View() string {
	return "tab: switch pane  ctrl+c: quit"
}

// RenderSplitPane renders two panes side by side.
func RenderSplitPane(left, right string, width, height int, focus Pane) string {
	return left + right
}
