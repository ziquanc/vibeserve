package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestDashboard_Init(t *testing.T) {
	m := NewDashboardModel()
	if m.activePanel != PanelRoutes {
		t.Errorf("expected initial panel PanelRoutes, got %d", m.activePanel)
	}
}

func TestDashboard_PanelCycle(t *testing.T) {
	m := NewDashboardModel()
	m.focused = true
	m.SetSize(40, 40)

	// j moves down
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if m.activePanel != PanelDBState {
		t.Errorf("expected PanelDBState after j, got %d", m.activePanel)
	}

	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if m.activePanel != PanelHTTPTrace {
		t.Errorf("expected PanelHTTPTrace after j, got %d", m.activePanel)
	}

	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if m.activePanel != PanelSnapshots {
		t.Errorf("expected PanelSnapshots after j, got %d", m.activePanel)
	}

	// Wraps around
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if m.activePanel != PanelRoutes {
		t.Errorf("expected PanelRoutes after wrap, got %d", m.activePanel)
	}

	// k moves up (wraps)
	m, _ = m.Update(tea.KeyPressMsg{Text: "k"})
	if m.activePanel != PanelSnapshots {
		t.Errorf("expected PanelSnapshots after k, got %d", m.activePanel)
	}
}

func TestDashboard_PanelCycle_NotFocused(t *testing.T) {
	m := NewDashboardModel()
	m.focused = false
	m.SetSize(40, 40)

	// j should NOT cycle when not focused
	m, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	if m.activePanel != PanelRoutes {
		t.Errorf("expected panel unchanged when not focused, got %d", m.activePanel)
	}
}

func TestDashboard_UpdateRoutes(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(40, 40)

	man := &manifest.Manifest{
		Routes: []manifest.Route{
			{Method: "GET", Path: "/users", Script: "list_users"},
			{Method: "POST", Path: "/users", Script: "create_user"},
		},
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER"},
				{Name: "name", Type: "TEXT"},
			}},
		},
	}

	m.UpdateFromManifest(man)

	if len(m.routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(m.routes))
	}
	if m.routes[0].Method != "GET" {
		t.Errorf("expected first route GET, got %s", m.routes[0].Method)
	}

	if len(m.tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(m.tables))
	}
	if m.tables[0].Name != "users" {
		t.Errorf("expected table 'users', got %s", m.tables[0].Name)
	}
	if m.tables[0].Columns != 2 {
		t.Errorf("expected 2 columns, got %d", m.tables[0].Columns)
	}
}

func TestDashboard_AddTrace(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(40, 40)

	m.AddHTTPTrace(HTTPRequestMsg{Method: "GET", Path: "/users"})
	m.AddHTTPTrace(HTTPResponseMsg{Method: "GET", Path: "/users", StatusCode: 200})

	if len(m.traces) != 2 {
		t.Errorf("expected 2 traces, got %d", len(m.traces))
	}
	if !m.traces[0].IsRequest {
		t.Error("expected first trace to be a request")
	}
	if m.traces[1].StatusCode != 200 {
		t.Errorf("expected status 200, got %d", m.traces[1].StatusCode)
	}

	// Test capped at 50
	for i := 0; i < 60; i++ {
		m.AddHTTPTrace(HTTPRequestMsg{Method: "GET", Path: "/test"})
	}

	if len(m.traces) != 50 {
		t.Errorf("expected traces capped at 50, got %d", len(m.traces))
	}
}

func TestDashboard_ViewShowsRoutes(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(40, 40)

	man := &manifest.Manifest{
		Routes: []manifest.Route{
			{Method: "GET", Path: "/users", Script: "list_users"},
		},
	}
	m.UpdateFromManifest(man)

	view := m.View()

	if !strings.Contains(view, "Routes") {
		t.Error("expected 'Routes' panel title in view")
	}
	if !strings.Contains(view, "GET") {
		t.Error("expected 'GET' in dashboard view")
	}
	if !strings.Contains(view, "/users") {
		t.Error("expected '/users' in dashboard view")
	}
}

func TestDashboard_View_Empty(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(40, 40)

	view := m.View()

	if !strings.Contains(view, "Routes") {
		t.Error("expected 'Routes' panel title in view")
	}
	if !strings.Contains(view, "DB State") {
		t.Error("expected 'DB State' panel title in view")
	}
	if !strings.Contains(view, "HTTP Trace") {
		t.Error("expected 'HTTP Trace' panel title in view")
	}
	if !strings.Contains(view, "Snapshots") {
		t.Error("expected 'Snapshots' panel title in view")
	}
	if !strings.Contains(view, "No routes") {
		t.Error("expected 'No routes' placeholder in empty view")
	}
}

func TestDashboard_View_ZeroSize(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(0, 0)

	view := m.View()
	if view != "" {
		t.Error("expected empty string for zero-size dashboard")
	}
}

func TestDashboard_ClearFlash(t *testing.T) {
	m := NewDashboardModel()
	m.routes = []routeDisplay{{Method: "GET", Path: "/test", flash: true}}
	m.tables = []tableDisplay{{Name: "test", Columns: 1, flash: true}}
	m.snapshots = []snapshotDisplay{{ID: 1, Description: "test", flash: true}}

	m.clearFlash()

	if m.routes[0].flash {
		t.Error("expected route flash cleared")
	}
	if m.tables[0].flash {
		t.Error("expected table flash cleared")
	}
	if m.snapshots[0].flash {
		t.Error("expected snapshot flash cleared")
	}
}

func TestDashboard_SetSnapshots(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(40, 40)

	snaps := []SnapshotInfo{
		{ID: 1, Description: "before_schema"},
		{ID: 2, Description: "after_migration"},
	}
	m.SetSnapshots(snaps)

	if len(m.snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(m.snapshots))
	}
	if m.snapshots[0].ID != 1 {
		t.Errorf("expected snapshot ID 1, got %d", m.snapshots[0].ID)
	}
	if m.snapshots[1].Description != "after_migration" {
		t.Errorf("expected description 'after_migration', got %s", m.snapshots[1].Description)
	}
}

func TestDashboard_AddSnapshot(t *testing.T) {
	m := NewDashboardModel()
	m.SetSize(40, 40)

	m.AddSnapshot(SnapshotCreatedMsg{ID: 1, Description: "before_schema"})
	m.AddSnapshot(SnapshotRestoredMsg{ID: 1, Description: "before_schema"})

	if len(m.snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(m.snapshots))
	}
	if m.snapshots[0].ID != 1 {
		t.Errorf("expected snapshot ID 1, got %d", m.snapshots[0].ID)
	}
	if !strings.Contains(m.snapshots[1].Description, "restored") {
		t.Error("expected restored snapshot to contain 'restored' in description")
	}
}
