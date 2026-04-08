package tui

import (
	lipgloss "charm.land/lipgloss/v2"
)

const (
	headerHeight    = 1
	statusBarHeight = 1
)

// Color palette
var (
	colorPrimary    = lipgloss.Color("#7C3AED") // violet
	colorSecondary  = lipgloss.Color("#06B6D4") // cyan
	colorSuccess    = lipgloss.Color("#22C55E") // green
	colorError      = lipgloss.Color("#EF4444") // red
	colorWarning    = lipgloss.Color("#F59E0B") // amber
	colorMuted      = lipgloss.Color("#6B7280") // gray
	colorText       = lipgloss.Color("#E5E7EB") // light gray
	colorBg         = lipgloss.Color("#111827") // dark background
	colorBorderDim  = lipgloss.Color("#374151") // dim border
	colorBorderHot  = lipgloss.Color("#7C3AED") // focused border
	colorFlashGreen = lipgloss.Color("#4ADE80") // flash highlight for new items
)

// Component styles
var (
	styleHeader = lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Foreground(colorMuted).
			Padding(0, 1)

	stylePanelTitle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	styleBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderHot)

	styleBorderDim = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorderDim)

	styleUserMsg = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	styleAssistantMsg = lipgloss.NewStyle().
				Foreground(colorText)

	styleSystemMsg = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	styleErrorMsg = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	stylePrompt = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	styleFlash = lipgloss.NewStyle().
			Foreground(colorFlashGreen).
			Bold(true)

	styleRoute = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleHTTPMethod = lipgloss.NewStyle().
			Foreground(colorWarning).
			Bold(true)

	styleHTTPPath = lipgloss.NewStyle().
			Foreground(colorText)

	styleSnapshotID = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	styleTableName = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)
)
