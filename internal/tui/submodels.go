package tui

import (
	"fmt"

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
		{"/help", "commands"},
		{"/undo", "rollback"},
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

