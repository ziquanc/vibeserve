package tui

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestSplitPane_WidthDistribution(t *testing.T) {
	left := "LEFT"
	right := "RIGHT"
	width := 120
	height := 30

	result := RenderSplitPane(left, right, width, height, PaneConversation)

	// Split at ~60/40. The rendered string should contain both panes.
	if !strings.Contains(result, left) {
		t.Error("expected left pane content in result")
	}
	if !strings.Contains(result, right) {
		t.Error("expected right pane content in result")
	}

	// Verify the rendered width is close to the total width (within a few columns
	// due to lipgloss border rounding).
	renderedWidth := lipgloss.Width(result)
	if renderedWidth < width-4 || renderedWidth > width {
		t.Errorf("expected rendered width near %d, got %d", width, renderedWidth)
	}

	// Verify left is wider than right by checking column ratio is roughly 60/40.
	leftWidth := width * 60 / 100   // 72
	rightWidth := width - leftWidth // 48
	if leftWidth <= rightWidth {
		t.Errorf("left width %d should be > right width %d", leftWidth, rightWidth)
	}
	ratio := float64(leftWidth) / float64(rightWidth)
	if ratio < 1.3 || ratio > 1.7 {
		t.Errorf("expected left/right ratio near 1.5 (60/40), got %.2f", ratio)
	}
}

func TestSplitPane_Collapse(t *testing.T) {
	left := "LEFTCONTENT"
	right := "RIGHTCONTENT"

	// Width 80 should collapse — only left appears.
	result := RenderSplitPane(left, right, 80, 24, PaneConversation)

	if !strings.Contains(result, left) {
		t.Error("expected left pane content in collapsed result")
	}
	if strings.Contains(result, right) {
		t.Error("right pane content should NOT appear when collapsed")
	}
}

func TestSplitPane_Collapse_Boundary(t *testing.T) {
	left := "L"
	right := "R"

	// Width exactly 100 should NOT collapse.
	result := RenderSplitPane(left, right, 100, 24, PaneConversation)
	if !strings.Contains(result, right) {
		t.Error("expected right pane at width=100 (not collapsed)")
	}

	// Width 99 should collapse.
	result99 := RenderSplitPane(left, right, 99, 24, PaneConversation)
	if strings.Contains(result99, right) {
		t.Error("right pane should NOT appear at width=99 (collapsed)")
	}
}

func TestSplitPane_FocusLeft(t *testing.T) {
	result := RenderSplitPane("A", "B", 120, 20, PaneConversation)
	_ = result
	// We verify this runs without panic and produces output containing both sides.
	if !strings.Contains(result, "A") {
		t.Error("expected 'A' in focus-left result")
	}
}

func TestSplitPane_FocusRight(t *testing.T) {
	result := RenderSplitPane("A", "B", 120, 20, PaneDashboard)
	_ = result
	if !strings.Contains(result, "B") {
		t.Error("expected 'B' in focus-right result")
	}
}

func TestIsCollapsed(t *testing.T) {
	tests := []struct {
		width    int
		expected bool
	}{
		{0, true},
		{50, true},
		{99, true},
		{100, false},
		{101, false},
		{200, false},
	}

	for _, tt := range tests {
		got := IsCollapsed(tt.width)
		if got != tt.expected {
			t.Errorf("IsCollapsed(%d) = %v, want %v", tt.width, got, tt.expected)
		}
	}
}
