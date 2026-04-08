package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// HeaderModel renders the top bar with project name, server URL, and help hint.
type HeaderModel struct {
	width     int
	serverURL string
}

// NewHeaderModel creates a HeaderModel with the given server URL.
func NewHeaderModel(serverURL string) HeaderModel {
	return HeaderModel{serverURL: serverURL}
}

// View renders the header bar.
func (m HeaderModel) View() string {
	left := styleHeader.Render(" VibeServe ")
	middle := lipgloss.NewStyle().
		Foreground(colorText).
		Background(lipgloss.Color("#1F2937")).
		Render(fmt.Sprintf(" %s ", m.serverURL))
	right := lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(lipgloss.Color("#1F2937")).
		Render(" Tab:switch  Ctrl+C:quit ")

	// Fill remaining width with background
	contentWidth := lipgloss.Width(left) + lipgloss.Width(middle) + lipgloss.Width(right)
	gap := m.width - contentWidth
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().
		Background(lipgloss.Color("#1F2937")).
		Render(fmt.Sprintf("%*s", gap, ""))

	return left + middle + filler + right
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

// StatusBarModel renders keybind hints at the bottom.
type StatusBarModel struct {
	width int
}

// NewStatusBarModel creates a StatusBarModel.
func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{}
}

// View renders the status bar.
func (m StatusBarModel) View() string {
	keys := []struct {
		key  string
		desc string
	}{
		{"Enter", "send"},
		{"Tab", "switch pane"},
		{"j/k", "cycle panels"},
		{"Ctrl+C", "quit"},
	}

	var parts string
	for i, k := range keys {
		key := lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true).
			Render(k.key)
		desc := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(k.desc)
		if i > 0 {
			parts += "  "
		}
		parts += key + " " + desc
	}

	content := styleStatusBar.Render(parts)

	// Fill to full width
	contentWidth := lipgloss.Width(content)
	gap := m.width - contentWidth
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().
		Background(lipgloss.Color("#1F2937")).
		Render(fmt.Sprintf("%*s", gap, ""))

	return content + filler
}

// RenderSplitPane renders two panes side by side.
func RenderSplitPane(left, right string, width, height int, focus Pane) string {
	return left + right
}
