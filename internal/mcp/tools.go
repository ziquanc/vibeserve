package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/export"
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

// handleQueryData executes a read-only SQL query.
func (srv *Server) handleQueryData(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()

	sqlStr, _ := args["sql"].(string)
	if sqlStr == "" {
		return errorResult("sql parameter is required"), nil
	}

	trimmed := strings.TrimSpace(sqlStr)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 || strings.ToUpper(fields[0]) != "SELECT" {
		return errorResult("Only SELECT queries are allowed. Use insert_data for writes."), nil
	}

	var params []any
	if rawParams, ok := args["params"]; ok {
		if arr, ok := rawParams.([]any); ok {
			params = arr
		}
	}

	rows, err := srv.store.Query(sqlStr, params)
	if err != nil {
		return errorResult(fmt.Sprintf("Query failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(rows, "", "  ")
	return mcplib.NewToolResultText(string(data)), nil
}

// handleInsertData inserts a row into a table.
func (srv *Server) handleInsertData(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()

	table, _ := args["table"].(string)
	if table == "" {
		return errorResult("table parameter is required"), nil
	}

	rawData, ok := args["data"]
	if !ok {
		return errorResult("data parameter is required"), nil
	}
	data, ok := rawData.(map[string]any)
	if !ok {
		return errorResult("data must be a JSON object"), nil
	}

	row, err := srv.store.Insert(table, data)
	if err != nil {
		return errorResult(fmt.Sprintf("Insert failed: %v", err)), nil
	}

	result, _ := json.MarshalIndent(row, "", "  ")
	return mcplib.NewToolResultText(string(result)), nil
}

// handleCreateAPI creates an API from a natural language description.
func (srv *Server) handleCreateAPI(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if srv.provider == nil {
		return errorResult("No LLM provider configured. Run 'vibeserve' once to set up your AI provider."), nil
	}

	args := req.GetArguments()
	description, _ := args["description"].(string)
	if description == "" {
		return errorResult("description parameter is required"), nil
	}

	result, err := srv.eng.ApplyAutoApprove(ctx, description)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to create API: %v", err)), nil
	}

	if result.ChatResponse != "" {
		return mcplib.NewToolResultText(result.ChatResponse), nil
	}

	return formatApplyResult(result), nil
}

// handleAddFeature adds a feature to the existing API.
func (srv *Server) handleAddFeature(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if srv.provider == nil {
		return errorResult("No LLM provider configured. Run 'vibeserve' once to set up your AI provider."), nil
	}

	args := req.GetArguments()
	description, _ := args["description"].(string)
	if description == "" {
		return errorResult("description parameter is required"), nil
	}

	m := srv.eng.Manifest()
	if m == nil || len(m.Schemas) == 0 {
		return errorResult("No API exists yet. Use create_api first."), nil
	}

	result, err := srv.eng.ApplyAutoApprove(ctx, description)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to add feature: %v", err)), nil
	}

	if result.ChatResponse != "" {
		return mcplib.NewToolResultText(result.ChatResponse), nil
	}

	return formatApplyResult(result), nil
}

// handleUndo rolls back the last schema change.
func (srv *Server) handleUndo(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	err := srv.eng.Undo()
	if err != nil {
		return errorResult(fmt.Sprintf("Undo failed: %v", err)), nil
	}
	return mcplib.NewToolResultText("Undo successful. Last change has been rolled back."), nil
}

// handleExportProject exports the API as a standalone project.
func (srv *Server) handleExportProject(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	m := srv.eng.Manifest()
	if m == nil || len(m.Schemas) == 0 {
		return errorResult("No API to export. Use create_api first."), nil
	}

	args := req.GetArguments()

	format, _ := args["format"].(string)
	if format == "" {
		format = "go"
	}
	if format != "go" && format != "express" && format != "next" && format != "fullstack" {
		return errorResult(fmt.Sprintf("Unsupported format %q. Use 'go', 'express', 'next', or 'fullstack'.", format)), nil
	}

	dbType, _ := args["db"].(string)
	if dbType == "" {
		dbType = "sqlite"
	}

	outDir, _ := args["output_dir"].(string)
	if outDir == "" {
		outDir = export.DefaultOutputDir(m)
	}

	exp := export.NewExporter(m, outDir)
	exp.SetVibeDir(srv.vibeDir)
	if dbType != "" {
		exp.SetDBType(dbType)
	}

	var exportErr error
	switch format {
	case "express":
		exportErr = exp.RunExpress()
	case "next":
		exportErr = exp.RunNext()
	case "fullstack":
		exportErr = exp.RunFullstack()
	default:
		exportErr = exp.Run()
	}
	if exportErr != nil {
		return errorResult(fmt.Sprintf("Export failed: %v", exportErr)), nil
	}

	return mcplib.NewToolResultText(fmt.Sprintf("Exported %s project to %s (format: %s, db: %s)", m.Name, outDir, format, dbType)), nil
}

// formatApplyResult formats an ApplyResult into readable MCP text.
func formatApplyResult(result *engine.ApplyResult) *mcplib.CallToolResult {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Applied %d changes:\n\n", len(result.Changes)))
	for _, c := range result.Changes {
		b.WriteString(fmt.Sprintf("  - [%s] %s\n", c.Type, c.Detail))
	}
	if len(result.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range result.Warnings {
			b.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}
	return mcplib.NewToolResultText(b.String())
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
