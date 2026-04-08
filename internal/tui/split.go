package tui

import (
	lipgloss "charm.land/lipgloss/v2"
)

// IsCollapsed returns true when the terminal width is too narrow to show both panes.
func IsCollapsed(width int) bool {
	return width < 100
}

// RenderSplitPane renders left and right panes side by side with rounded borders.
//
// Layout rules:
//   - Left pane gets 60% of total width, right gets 40%.
//   - The focused pane has a violet border; the unfocused pane uses a dim border.
//   - When width < 100 (collapsed), only the left pane is returned at full width.
func RenderSplitPane(left, right string, width, height int, focus Pane) string {
	if IsCollapsed(width) {
		// Collapsed: show only the left pane at full width.
		return styleBorderFocused.
			Width(width - 2). // subtract border characters
			Height(height - 2).
			Render(left)
	}

	leftWidth := width * 60 / 100
	rightWidth := width - leftWidth

	// Each border takes 2 columns (left + right side), so inner width = paneWidth - 2.
	leftInner := leftWidth - 2
	rightInner := rightWidth - 2
	innerHeight := height - 2

	if leftInner < 1 {
		leftInner = 1
	}
	if rightInner < 1 {
		rightInner = 1
	}
	if innerHeight < 1 {
		innerHeight = 1
	}

	leftBorderColor := colorBorderDim
	rightBorderColor := colorBorderDim
	if focus == PaneConversation {
		leftBorderColor = colorBorderHot
	} else {
		rightBorderColor = colorBorderHot
	}

	leftRendered := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(leftBorderColor).
		Width(leftInner).
		Height(innerHeight).
		Render(left)

	rightRendered := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorderColor).
		Width(rightInner).
		Height(innerHeight).
		Render(right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, rightRendered)
}
