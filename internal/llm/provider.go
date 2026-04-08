package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Message represents a single conversation message.
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
}

// Provider generates a new manifest from a user prompt and conversation history.
type Provider interface {
	// Generate sends the user prompt (with history and current manifest context)
	// to the LLM and returns the generated manifest.
	Generate(ctx context.Context, current *manifest.Manifest, prompt string, history []Message) (*manifest.Manifest, error)
}

// BuildSystemPrompt constructs the system prompt that instructs the LLM how to
// produce valid manifests. It includes:
// 1. The manifest JSON schema with an example
// 2. All stdlib functions (with Tengo keyword fixes: response.fail, log.err)
// 3. The current manifest as context
// 4. The Migration Memory Rule
// 5. Instruction to output ONLY valid JSON
func BuildSystemPrompt(current *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString(`You are VibeServe, an AI backend generator. You produce JSON manifests that define APIs.

## Manifest JSON Schema

A manifest is a JSON object with these fields:

{
  "version": "1.0",
  "name": "my-api",
  "description": "Description of the API",
  "schemas": [
    {
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "email", "type": "TEXT", "required": true, "unique": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "active", "type": "BOOLEAN", "default": true},
        {"name": "score", "type": "REAL"},
        {"name": "created_at", "type": "DATETIME", "default": "NOW"},
        {"name": "birth_date", "type": "DATE"},
        {"name": "category_id", "type": "INTEGER", "references": "categories.id"}
      ]
    }
  ],
  "routes": [
    {
      "path": "/users",
      "method": "GET",
      "description": "List all users",
      "script": "list_users",
      "response_type": "array"
    },
    {
      "path": "/users/:id",
      "method": "GET",
      "description": "Get a user by ID",
      "script": "get_user",
      "response_type": "object"
    },
    {
      "path": "/users",
      "method": "POST",
      "description": "Create a user",
      "script": "create_user",
      "request_body": {"email": "TEXT", "name": "TEXT"},
      "response_type": "object"
    }
  ],
  "scripts": [
    {
      "name": "list_users",
      "code": "result := db.query(\"SELECT * FROM users\", [])\nresponse.json(result)"
    }
  ],
  "seeds": [
    {
      "table": "users",
      "rows": [
        {"email": "alice@example.com", "name": "Alice"}
      ]
    }
  ]
}

### Column Types
Valid column types: INTEGER, TEXT, REAL, BOOLEAN, DATE, DATETIME

### Route Methods
Valid methods: GET, POST, PUT, PATCH, DELETE

### Path Parameters
Use :param syntax for dynamic segments: /users/:id, /posts/:postId/comments/:commentId

## Tengo Script Standard Library

Scripts are written in Tengo. These modules are injected automatically:

### db — Database operations
- db.query(sql, params) → array of maps (or error)
- db.query_one(sql, params) → map or undefined (or error)
- db.insert(table, data) → inserted row map (or error)
- db.update(table, id, data) → updated row map (or error)
- db.delete(table, id) → true/false (or error)
- db.count(table) → integer (or error)

Parameters are passed as arrays: db.query("SELECT * FROM users WHERE id = ?", [id])

### request — HTTP request data
- request.param(name) → string or undefined (path parameter)
- request.query(name) → string or undefined (query string)
- request.body() → map or undefined (parsed JSON body)
- request.header(name) → string or undefined
- request.method() → string ("GET", "POST", etc.)
- request.auth() → map (decoded Bearer token payload) or undefined

### response — HTTP response
- response.json(data) → sends JSON with status 200
- response.json(data, status) → sends JSON with custom status code
- response.fail(status, message) → sends {"error": message} with status code
- response.header(name, value) → sets a response header
- response.redirect(url) → sends 302 redirect

IMPORTANT: "error" is a reserved keyword in Tengo. Use response.fail() — never use response dot error.

### date — Date utilities
- date.now() → current UTC datetime as RFC3339 string
- date.diff_days(dateA, dateB) → integer days between dates
- date.add_days(date, n) → new date string
- date.format(date, layout) → formatted string (Go layout syntax)

### crypto — Security utilities
- crypto.hash(str) → SHA-256 hex string
- crypto.uuid() → random UUID v4 string
- crypto.random(min, max) → random integer in range [min, max]

### log — Logging
- log.info(message) → log at info level
- log.warn(message) → log at warn level
- log.err(message) → log at error level

IMPORTANT: "error" is a reserved keyword in Tengo. Use log.err() — never use log dot error.

## Script Examples

List with filtering:
result := db.query("SELECT * FROM items WHERE active = ?", [true])
response.json(result)

Get by ID with 404 handling:
id := request.param("id")
row := db.query_one("SELECT * FROM items WHERE id = ?", [id])
if row == undefined {
  response.fail(404, "Not found")
} else {
  response.json(row)
}

Create with validation:
body := request.body()
if body.name == undefined {
  response.fail(400, "name is required")
}
result := db.insert("items", {
  name: body.name,
  created_at: date.now()
})
response.json(result, 201)

Update:
id := request.param("id")
body := request.body()
result := db.update("items", id, body)
response.json(result)

Delete:
id := request.param("id")
deleted := db.delete("items", id)
if deleted {
  response.json({message: "deleted"})
} else {
  response.fail(404, "Not found")
}

Query parameter filtering:
status := request.query("status")
if status == undefined {
  result := db.query("SELECT * FROM tasks", [])
  response.json(result)
} else {
  result := db.query("SELECT * FROM tasks WHERE status = ?", [status])
  response.json(result)
}

`)

	b.WriteString("## Migration Memory Rule\n\n")
	b.WriteString("CRITICAL: You must NEVER rename or remove primary key columns (columns with \"primary\": true).\n")
	b.WriteString("You must NEVER rename or remove foreign key columns (columns with \"references\").\n")
	b.WriteString("You may ADD new columns to existing tables.\n")
	b.WriteString("You may ADD new tables.\n")
	b.WriteString("You may ADD new routes, scripts, and seeds.\n")
	b.WriteString("You may UPDATE existing scripts (change their code).\n")
	b.WriteString("You may REMOVE routes and their scripts if the user asks.\n")
	b.WriteString("If you remove a column from the schema, it will be IGNORED (SQLite cannot drop columns in older versions).\n")
	b.WriteString("Always include ALL existing tables and columns in your output, even if unchanged.\n\n")

	if current != nil {
		b.WriteString("## Current Manifest\n\n")
		b.WriteString("This is the current state of the API. Build upon it — do not start from scratch.\n\n")
		b.WriteString("```json\n")
		data, err := json.MarshalIndent(current, "", "  ")
		if err == nil {
			b.Write(data)
		}
		b.WriteString("\n```\n\n")
	} else {
		b.WriteString("## Current State\n\n")
		b.WriteString("No existing manifest. Create a new one from scratch based on the user's request.\n\n")
	}

	b.WriteString("## Output Rules\n\n")
	b.WriteString("When the user asks you to CREATE, MODIFY, ADD, UPDATE, or DELETE an API, table, route, or feature:\n")
	b.WriteString("1. Output ONLY valid JSON — no markdown fences, no explanation, no commentary.\n")
	b.WriteString("2. The JSON must be a complete manifest object with all required fields.\n")
	b.WriteString("3. Include ALL existing schemas, routes, scripts, and seeds, plus any changes.\n")
	b.WriteString("4. Every route must reference a script that exists in the scripts array.\n")
	b.WriteString("5. Every seed must reference a table that exists in the schemas array.\n")
	b.WriteString("6. Script code must be valid Tengo. Use response.fail() and log.err() — never use the reserved 'error' keyword as a function name.\n\n")
	b.WriteString("When the user asks a QUESTION (e.g., 'what can you do?', 'how does this work?', 'what is your model?'):\n")
	b.WriteString("Respond with a helpful text answer. Do NOT output JSON for questions — just answer naturally.\n")

	return b.String()
}

// BuildPlanPrompt creates a prompt that asks the LLM to break a request into steps.
func BuildPlanPrompt(userRequest string) string {
	return fmt.Sprintf(`The user wants: "%s"

Break this into 2-5 small implementation steps. Each step should add ONE thing (a table, a few related routes, seed data, etc.).

Output ONLY a JSON array of step descriptions. Example:
["Create users table with id, name, email columns", "Create posts table with id, title, body, user_id columns", "Add CRUD routes for users", "Add CRUD routes for posts", "Add seed data for users and posts"]

Rules:
- Each step should be small enough to implement independently
- Start with tables/schemas, then routes, then seed data
- Each step description should be specific and actionable
- Output ONLY the JSON array, no other text`, userRequest)
}

// ExtractPlan parses a JSON array of step descriptions from LLM output.
func ExtractPlan(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)

	// Find the array
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in plan response")
	}

	var steps []string
	if err := json.Unmarshal([]byte(raw[start:end+1]), &steps); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("plan has no steps")
	}
	return steps, nil
}

// ExtractJSON finds and extracts a JSON object from LLM output.
// Some LLMs wrap JSON in markdown fences or add explanation text.
// This function attempts to find the outermost {...} in the response.
func ExtractJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	// If it already starts with {, try it directly
	if strings.HasPrefix(raw, "{") {
		return raw, nil
	}

	// Try to find JSON within markdown fences
	if idx := strings.Index(raw, "```json"); idx != -1 {
		start := idx + len("```json")
		end := strings.Index(raw[start:], "```")
		if end != -1 {
			return strings.TrimSpace(raw[start : start+end]), nil
		}
	}
	if idx := strings.Index(raw, "```"); idx != -1 {
		start := idx + len("```")
		end := strings.Index(raw[start:], "```")
		if end != -1 {
			candidate := strings.TrimSpace(raw[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate, nil
			}
		}
	}

	// Find the first { and last }
	first := strings.Index(raw, "{")
	last := strings.LastIndex(raw, "}")
	if first != -1 && last > first {
		return raw[first : last+1], nil
	}

	// Show a preview of what the LLM returned for debugging
	preview := raw
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return "", fmt.Errorf("no JSON object found in LLM response: %q", preview)
}

// ChatOnlyError is returned when the LLM response contains no JSON manifest,
// only conversational text. The Text field contains the LLM's response.
type ChatOnlyError struct {
	Text string
}

func (e *ChatOnlyError) Error() string {
	return "LLM returned conversational text, not a manifest"
}

// ParseManifestResponse extracts and parses a manifest from raw LLM text output.
// If no JSON is found, returns a ChatOnlyError containing the raw text.
func ParseManifestResponse(raw string) (*manifest.Manifest, error) {
	jsonStr, err := ExtractJSON(raw)
	if err != nil {
		return nil, &ChatOnlyError{Text: raw}
	}

	var m manifest.Manifest
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil, &ChatOnlyError{Text: raw}
	}

	return &m, nil
}
