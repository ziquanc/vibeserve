# VibeServe MCP Server — Design Spec

## Goal

Expose VibeServe as an MCP (Model Context Protocol) server so AI coding assistants (Claude Code, Cursor, Windsurf) and custom agents can programmatically create, modify, query, and export APIs.

## Architecture

`vibeserve mcp` starts a stdio-based MCP server. It creates its own Engine instance operating directly on the `.vibe/` directory — the same approach the TUI uses. No HTTP dependency. The running `vibeserve` server (if any) picks up changes automatically since both share the same `manifest.json` and `state.db`.

```
Claude Code / Cursor
    | stdio (MCP protocol)
vibeserve mcp
    | direct Go calls
Engine -> .vibe/manifest.json + state.db
    | (same files)
vibeserve (main server, running separately)
```

### MCP Configuration

Users add this to their Claude Code / Cursor MCP config:

```json
{
  "mcpServers": {
    "vibeserve": {
      "command": "vibeserve",
      "args": ["mcp"]
    }
  }
}
```

## Tools

9 tools organized by purpose:

### Schema & Route Management

**`create_api`**
- Input: `{ description: string }`
- Description: "Create an API from a natural language description. Generates database tables, REST routes, business logic scripts, and seed data. Example: 'Create a task management API with users, projects, and tasks. Tasks have priorities and due dates.'"
- Implementation: Call `Engine.Apply(ctx, description)` with auto-approve. Return the list of changes applied (tables created, routes added).
- Error: If no LLM provider is configured, return error with setup instructions.

**`add_feature`**
- Input: `{ description: string }`
- Description: "Add a feature to the existing API. Can add new tables, columns, routes, or modify existing behavior. Example: 'Add a comments feature to tasks with author and timestamp.'"
- Implementation: Same as `create_api` — calls `Engine.Apply()`. The engine handles both initial creation and incremental changes.
- Error: If no manifest exists, suggest using `create_api` first.

**`undo`**
- Input: (none)
- Description: "Undo the last schema change. Restores the database and manifest to the previous snapshot."
- Implementation: Call `Engine.Undo()`.

### Inspection

**`list_routes`**
- Input: (none)
- Description: "List all API routes with their HTTP method, path, and description."
- Implementation: Read `Engine.Manifest().Routes`. Return as formatted table.
- Output: Array of `{ method, path, description }`.

**`list_tables`**
- Input: (none)
- Description: "List all database tables with their columns, types, constraints, and row counts."
- Implementation: Read `Engine.Manifest().Schemas` for column definitions. Call `Store.Count()` for row counts.
- Output: Array of `{ table, columns: [{ name, type, primary, required, unique, references }], row_count }`.

**`get_api_status`**
- Input: (none)
- Description: "Show the current API status: number of tables, routes, manifest path, and database path."
- Implementation: Read manifest and return summary counts.
- Output: `{ tables, routes, scripts, manifest_path, db_path }`.

### Data

**`query_data`**
- Input: `{ sql: string, params?: any[] }`
- Description: "Execute a read-only SQL query against the API database. Only SELECT statements are allowed."
- Implementation: Validate SQL starts with SELECT (case-insensitive, trimmed). Call `Store.Query()`.
- Error: Reject non-SELECT statements with "Only SELECT queries are allowed. Use insert_data for writes."

**`insert_data`**
- Input: `{ table: string, data: object }`
- Description: "Insert a row into a database table. Returns the inserted row with generated ID."
- Implementation: Validate table name. Call `Store.Insert()`.

### Export

**`export_project`**
- Input: `{ format: "go" | "express", db?: "sqlite" | "postgres", output_dir?: string }`
- Description: "Export the API as a standalone production project. Supports Go (Chi router) and Express.js (with SQLite or PostgreSQL)."
- Implementation: Create Exporter, set format and db type, call `Run()` or `RunExpress()`. Default output_dir: `vibe-export-{name}`.
- Output: `{ output_dir, format, db, files_generated }`.

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `internal/mcp/server.go` | MCP server setup, tool registration, stdio transport |
| `internal/mcp/tools.go` | Tool handler implementations (one function per tool) |
| `internal/mcp/server_test.go` | Tests for tool handlers |

### Modified files

| File | Changes |
|------|---------|
| `cmd/vibeserve/main.go` | Add `mcp` cobra subcommand |
| `go.mod` | Add `github.com/mark3labs/mcp-go` dependency |

## Implementation Details

### MCP Server Setup (`server.go`)

```go
type Server struct {
    engine   *Engine
    store    *store.Store
    manifest *manifest.Manifest
    vibeDir  string
    provider llm.Provider
}
```

The server:
1. Loads config from `.vibe/config.yaml`
2. Opens the store (`.vibe/state.db`)
3. Creates an Engine with the store and LLM provider
4. Loads the existing manifest (if any)
5. Registers all 9 tools with the MCP SDK
6. Starts stdio transport

### Tool Registration

Each tool is registered with the MCP SDK using `mcp.NewTool()` with a JSON Schema input definition and a handler function. The handler receives parsed arguments and returns a text result (MCP tools return text content).

### Error Handling

- **No `.vibe/` directory**: Tools that need it return a clear error: "No VibeServe project found in the current directory. Run 'vibeserve' first to create an API, or use create_api to start."
- **No LLM configured**: `create_api` and `add_feature` return: "No LLM provider configured. Run 'vibeserve' once to set up your AI provider."
- **Query validation**: `query_data` trims and upper-cases the first word. If not "SELECT", reject.
- **Table validation**: `insert_data` validates table name with existing `ValidateTableName()`.
- **LLM failures**: Return the error detail so the AI assistant can inform the user.

### Dependencies

- `github.com/mark3labs/mcp-go` — Go MCP SDK (stdio transport, tool registration, JSON-RPC handling)
- All existing VibeServe packages (engine, store, manifest, export, config, llm)

## Testing

- Unit tests for each tool handler with a test Engine (in-memory SQLite)
- Test that `query_data` rejects non-SELECT
- Test that `insert_data` validates table names
- Test tool registration (all 9 tools registered)
- Integration test: create_api -> list_tables -> query_data -> export_project flow

## Out of Scope

- MCP resources (read-only data endpoints) — tools are sufficient for v1
- MCP prompts (prompt templates) — not needed
- Server management (start/stop the main vibeserve server) — user manages this separately
- Authentication — MCP runs locally via stdio, no auth needed
