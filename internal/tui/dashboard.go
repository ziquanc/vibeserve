package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/snapshot"
)

// PanelID identifies a dashboard panel.
type PanelID int

const (
	PanelRoutes PanelID = iota
	PanelDBState
	PanelHTTPTrace
	PanelSnapshots
	panelCount = 4
)

// HTTPTraceEntry records a single HTTP request or response event.
type HTTPTraceEntry struct {
	Time       time.Time
	Method     string
	Path       string
	StatusCode int
	IsRequest  bool
}

// DashboardModel manages the four right-side panels.
type DashboardModel struct {
	width       int
	height      int
	focused     bool
	activePanel PanelID

	// Data
	routes    []routeDisplay
	tables    []tableDisplay
	traces    []HTTPTraceEntry
	snapshots []snapshotDisplay
	events    []string // recent schema/route events for flash
}

type routeDisplay struct {
	Method string
	Path   string
	Script string
	flash  bool
}

type tableDisplay struct {
	Name    string
	Columns int
	flash   bool
}

type snapshotDisplay struct {
	ID          int
	Description string
	flash       bool
}

// NewDashboardModel creates an empty DashboardModel.
func NewDashboardModel() DashboardModel {
	return DashboardModel{
		activePanel: PanelRoutes,
	}
}

// SetSize updates dashboard dimensions.
func (m *DashboardModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Update handles dashboard key events.
func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.focused {
			switch msg.String() {
			case "j", "down":
				m.activePanel = (m.activePanel + 1) % panelCount
			case "k", "up":
				m.activePanel = (m.activePanel - 1 + panelCount) % panelCount
			case "enter":
				// Could toggle expand/collapse in future; no-op for now
			}
		}
	case FlashClearMsg:
		m.clearFlash()
	}
	return m, nil
}

// UpdateFromManifest refreshes the routes and tables from the current manifest.
func (m *DashboardModel) UpdateFromManifest(man *manifest.Manifest) {
	if man == nil {
		return
	}

	// Update routes
	m.routes = make([]routeDisplay, len(man.Routes))
	for i, r := range man.Routes {
		m.routes[i] = routeDisplay{
			Method: r.Method,
			Path:   r.Path,
			Script: r.Script,
		}
	}

	// Update tables
	m.tables = make([]tableDisplay, len(man.Schemas))
	for i, s := range man.Schemas {
		m.tables[i] = tableDisplay{
			Name:    s.Table,
			Columns: len(s.Columns),
		}
	}
}

// AddRouteEvent records a route change event.
func (m *DashboardModel) AddRouteEvent(detail string) {
	m.events = append(m.events, detail)
	// Mark the last route as flashing
	if len(m.routes) > 0 {
		m.routes[len(m.routes)-1].flash = true
	}
}

// AddHTTPTrace adds an HTTP trace entry.
func (m *DashboardModel) AddHTTPTrace(msg tea.Msg) {
	switch msg := msg.(type) {
	case HTTPRequestMsg:
		m.traces = append(m.traces, HTTPTraceEntry{
			Time:      time.Now(),
			Method:    msg.Method,
			Path:      msg.Path,
			IsRequest: true,
		})
	case HTTPResponseMsg:
		m.traces = append(m.traces, HTTPTraceEntry{
			Time:       time.Now(),
			Method:     msg.Method,
			Path:       msg.Path,
			StatusCode: msg.StatusCode,
			IsRequest:  false,
		})
	}
	// Keep only the last 50 entries
	if len(m.traces) > 50 {
		m.traces = m.traces[len(m.traces)-50:]
	}
}

// AddSnapshot records a snapshot event.
func (m *DashboardModel) AddSnapshot(msg tea.Msg) {
	switch msg := msg.(type) {
	case SnapshotCreatedMsg:
		m.snapshots = append(m.snapshots, snapshotDisplay{
			ID:          msg.ID,
			Description: msg.Description,
			flash:       true,
		})
	case SnapshotRestoredMsg:
		m.snapshots = append(m.snapshots, snapshotDisplay{
			ID:          msg.ID,
			Description: "restored: " + msg.Description,
			flash:       true,
		})
	}
}

// AddSchemaEvent records a schema change event.
func (m *DashboardModel) AddSchemaEvent(detail string) {
	m.events = append(m.events, detail)
	if len(m.tables) > 0 {
		m.tables[len(m.tables)-1].flash = true
	}
}

// SetSnapshots updates the snapshot list directly.
func (m *DashboardModel) SetSnapshots(snaps []SnapshotInfo) {
	m.snapshots = make([]snapshotDisplay, len(snaps))
	for i, s := range snaps {
		m.snapshots[i] = snapshotDisplay{
			ID:          s.ID,
			Description: s.Description,
		}
	}
}

// SnapshotInfo holds display info for a snapshot.
type SnapshotInfo struct {
	ID          int
	Description string
}

// LoadSnapshots populates the snapshot list from disk.
func (m *DashboardModel) LoadSnapshots(vibeDir string) {
	snaps, err := snapshot.List(vibeDir)
	if err != nil {
		return
	}
	m.snapshots = make([]snapshotDisplay, len(snaps))
	for i, s := range snaps {
		m.snapshots[i] = snapshotDisplay{
			ID:          s.ID,
			Description: s.Description,
		}
	}
}

// View renders the dashboard.
func (m DashboardModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	contentWidth := m.width - 2 // account for border
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Each panel gets roughly 1/4 of the height
	panelHeight := m.height / panelCount
	if panelHeight < 3 {
		panelHeight = 3
	}

	panels := []string{
		m.renderRoutesPanel(contentWidth, panelHeight, m.activePanel == PanelRoutes),
		m.renderDBStatePanel(contentWidth, panelHeight, m.activePanel == PanelDBState),
		m.renderHTTPTracePanel(contentWidth, panelHeight, m.activePanel == PanelHTTPTrace),
		m.renderSnapshotsPanel(contentWidth, panelHeight, m.activePanel == PanelSnapshots),
	}

	joined := lipgloss.JoinVertical(lipgloss.Left, panels...)

	return joined
}

func (m DashboardModel) renderRoutesPanel(width, height int, active bool) string {
	title := stylePanelTitle.Render("Routes")
	if active && m.focused {
		title = stylePanelTitle.Render("> Routes")
	}

	var lines []string
	if len(m.routes) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorMuted).Render("  No routes"))
	} else {
		for _, r := range m.routes {
			method := styleHTTPMethod.Width(7).Render(r.Method)
			path := styleHTTPPath.Render(r.Path)
			line := fmt.Sprintf("  %s %s", method, path)
			if r.flash {
				line = styleFlash.Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := title + "\n" + strings.Join(lines, "\n")

	borderStyle := styleBorderDim
	if active && m.focused {
		borderStyle = styleBorderFocused
	}

	return borderStyle.
		Width(width).
		Height(height).
		Render(content)
}

func (m DashboardModel) renderDBStatePanel(width, height int, active bool) string {
	title := stylePanelTitle.Render("DB State")
	if active && m.focused {
		title = stylePanelTitle.Render("> DB State")
	}

	var lines []string
	if len(m.tables) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorMuted).Render("  No tables"))
	} else {
		for _, t := range m.tables {
			name := styleTableName.Render(t.Name)
			info := lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf(" (%d cols)", t.Columns))
			line := fmt.Sprintf("  %s%s", name, info)
			if t.flash {
				line = styleFlash.Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := title + "\n" + strings.Join(lines, "\n")

	borderStyle := styleBorderDim
	if active && m.focused {
		borderStyle = styleBorderFocused
	}

	return borderStyle.
		Width(width).
		Height(height).
		Render(content)
}

func (m DashboardModel) renderHTTPTracePanel(width, height int, active bool) string {
	title := stylePanelTitle.Render("HTTP Trace")
	if active && m.focused {
		title = stylePanelTitle.Render("> HTTP Trace")
	}

	var lines []string
	if len(m.traces) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorMuted).Render("  No requests yet"))
	} else {
		// Show most recent entries that fit
		maxLines := height - 2
		if maxLines < 1 {
			maxLines = 1
		}
		start := len(m.traces) - maxLines
		if start < 0 {
			start = 0
		}
		for _, tr := range m.traces[start:] {
			timestamp := tr.Time.Format("15:04:05")
			method := styleHTTPMethod.Width(7).Render(tr.Method)
			if tr.IsRequest {
				line := fmt.Sprintf("  %s %s %s", lipgloss.NewStyle().Foreground(colorMuted).Render(timestamp), method, tr.Path)
				lines = append(lines, line)
			} else {
				statusStyle := lipgloss.NewStyle().Foreground(colorSuccess)
				if tr.StatusCode >= 400 {
					statusStyle = lipgloss.NewStyle().Foreground(colorError)
				}
				status := statusStyle.Render(fmt.Sprintf("%d", tr.StatusCode))
				line := fmt.Sprintf("  %s %s %s %s",
					lipgloss.NewStyle().Foreground(colorMuted).Render(timestamp),
					method, tr.Path, status)
				lines = append(lines, line)
			}
		}
	}

	content := title + "\n" + strings.Join(lines, "\n")

	borderStyle := styleBorderDim
	if active && m.focused {
		borderStyle = styleBorderFocused
	}

	return borderStyle.
		Width(width).
		Height(height).
		Render(content)
}

func (m DashboardModel) renderSnapshotsPanel(width, height int, active bool) string {
	title := stylePanelTitle.Render("Snapshots")
	if active && m.focused {
		title = stylePanelTitle.Render("> Snapshots")
	}

	var lines []string
	if len(m.snapshots) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorMuted).Render("  No snapshots"))
	} else {
		// Show most recent first
		maxLines := height - 2
		if maxLines < 1 {
			maxLines = 1
		}
		start := len(m.snapshots) - maxLines
		if start < 0 {
			start = 0
		}
		for _, s := range m.snapshots[start:] {
			id := styleSnapshotID.Render(fmt.Sprintf("#%d", s.ID))
			line := fmt.Sprintf("  %s %s", id, s.Description)
			if s.flash {
				line = styleFlash.Render(line)
			}
			lines = append(lines, line)
		}
	}

	content := title + "\n" + strings.Join(lines, "\n")

	borderStyle := styleBorderDim
	if active && m.focused {
		borderStyle = styleBorderFocused
	}

	return borderStyle.
		Width(width).
		Height(height).
		Render(content)
}

func (m *DashboardModel) clearFlash() {
	for i := range m.routes {
		m.routes[i].flash = false
	}
	for i := range m.tables {
		m.tables[i].flash = false
	}
	for i := range m.snapshots {
		m.snapshots[i].flash = false
	}
}
