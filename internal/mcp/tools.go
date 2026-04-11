package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// handleListRoutes returns all API routes.
func (srv *Server) handleListRoutes(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	m := srv.eng.Manifest()
	if m == nil || len(m.Routes) == 0 {
		return mcplib.NewToolResultText("No routes defined. Use create_api to create an API."), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("API Routes (%d total):\n\n", len(m.Routes)))
	b.WriteString(fmt.Sprintf("%-8s %-30s %s\n", "Method", "Path", "Description"))
	b.WriteString(strings.Repeat("-", 70) + "\n")
	for _, r := range m.Routes {
		b.WriteString(fmt.Sprintf("%-8s %-30s %s\n", r.Method, r.Path, r.Description))
	}
	return mcplib.NewToolResultText(b.String()), nil
}

// handleListTables returns all tables with columns and row counts.
func (srv *Server) handleListTables(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	m := srv.eng.Manifest()
	if m == nil || len(m.Schemas) == 0 {
		return mcplib.NewToolResultText("No tables defined. Use create_api to create an API."), nil
	}

	type colInfo struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Primary    bool   `json:"primary,omitempty"`
		Required   bool   `json:"required,omitempty"`
		Unique     bool   `json:"unique,omitempty"`
		References string `json:"references,omitempty"`
	}
	type tableInfo struct {
		Table    string    `json:"table"`
		Columns  []colInfo `json:"columns"`
		RowCount int       `json:"row_count"`
	}

	var tables []tableInfo
	for _, s := range m.Schemas {
		count, _ := srv.store.Count(s.Table)
		var cols []colInfo
		for _, c := range s.Columns {
			cols = append(cols, colInfo{
				Name:       c.Name,
				Type:       c.Type,
				Primary:    c.Primary,
				Required:   c.Required,
				Unique:     c.Unique,
				References: c.References,
			})
		}
		tables = append(tables, tableInfo{
			Table:    s.Table,
			Columns:  cols,
			RowCount: count,
		})
	}

	data, _ := json.MarshalIndent(tables, "", "  ")
	return mcplib.NewToolResultText(string(data)), nil
}

// handleGetAPIStatus returns a summary of the API state.
func (srv *Server) handleGetAPIStatus(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	m := srv.eng.Manifest()

	tables := 0
	routes := 0
	scripts := 0
	name := ""
	if m != nil {
		tables = len(m.Schemas)
		routes = len(m.Routes)
		scripts = len(m.Scripts)
		name = m.Name
	}

	status := map[string]any{
		"name":     name,
		"tables":   tables,
		"routes":   routes,
		"scripts":  scripts,
		"vibe_dir": srv.vibeDir,
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	return mcplib.NewToolResultText(string(data)), nil
}

// errorResult creates an MCP error result.
func errorResult(msg string) *mcplib.CallToolResult {
	return &mcplib.CallToolResult{
		Content: []mcplib.Content{
			mcplib.TextContent{
				Type: "text",
				Text: msg,
			},
		},
		IsError: true,
	}
}
