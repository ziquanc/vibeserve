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
    },
    {
      "table": "orders",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "user_id", "type": "INTEGER", "references": "users.id"},
        {"name": "total", "type": "REAL"},
        {"name": "status", "type": "TEXT", "default": "draft"}
      ],
      "state_machine": {
        "field": "status",
        "initial": "draft",
        "transitions": [
          {"from": "draft", "to": "submitted", "action": "submit", "guard": {"condition": "total > 0"}},
          {"from": "submitted", "to": "approved", "action": "approve", "guard": {"role": "admin"}}
        ]
      }
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

	b.WriteString("## Timestamp Columns\n\n")
	b.WriteString("Every table MUST include these three columns:\n")
	b.WriteString("- created_at (DATETIME, default: \"NOW\") — set on insert\n")
	b.WriteString("- updated_at (DATETIME, default: \"NOW\") — set on insert and update\n")
	b.WriteString("- deleted_at (DATETIME) — NULL by default, set on soft delete\n")
	b.WriteString("If a table already has these columns, keep them. If not, add them.\n")
	b.WriteString("For delete scripts, use soft delete: UPDATE table SET deleted_at = date.now() WHERE id = ? instead of db.delete().\n")
	b.WriteString("For list scripts, always filter: SELECT * FROM table WHERE deleted_at IS NULL.\n\n")

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
	b.WriteString("Respond with a helpful text answer. Do NOT output JSON for questions — just answer naturally.\n\n")

	// ThinkStack cognitive modes — applied only during design tasks.
	// These rules improve schema quality, relationship modeling, and API architecture.
	// See: github.com/ziquanc/thinkstack
	b.WriteString("## Thinking Modes (apply these when designing, not when answering questions)\n\n")

	b.WriteString("### Systems Thinking\n")
	b.WriteString("Trace the ripple effects. Nothing exists in isolation.\n")
	b.WriteString("- Map the system first. Name the actors, components, and data flows before designing tables.\n")
	b.WriteString("- Find feedback loops. Where does output become input? A booking updates availability, availability constrains bookings.\n")
	b.WriteString("- State second-order effects. For every table/route, write: First-order: [direct]. Second-order: [what it causes].\n")
	b.WriteString("- Check adjacent systems. What other entities touch this one? Add foreign keys and relationship routes.\n\n")

	b.WriteString("### First-Principles\n")
	b.WriteString("Question the premise before designing.\n")
	b.WriteString("- Challenge the framing. 'Create a user table' might really mean 'create an auth system with roles and permissions.'\n")
	b.WriteString("- Separate constraints from conventions. What MUST be true vs what people assume? Not every entity needs full CRUD.\n")
	b.WriteString("- Name assumptions. List them explicitly. 'Assuming one user per email' — make it a UNIQUE constraint.\n")
	b.WriteString("- Rebuild from zero. What would the ideal schema look like with no legacy? Then design that.\n\n")

	b.WriteString("### Tradeoff\n")
	b.WriteString("Every design decision has a cost. Name it.\n")
	b.WriteString("- State what you give up. Denormalization speeds reads but complicates writes. Say so.\n")
	b.WriteString("- List alternatives. Before one big table, consider: separate tables with FKs? A junction table? An enum column?\n")
	b.WriteString("- Assess reversibility. Adding a column is easy. Splitting a table is hard. Design for the hard-to-reverse cases.\n")
	b.WriteString("- Name what you optimize for. 'This schema optimizes for read-heavy queries over write simplicity.'\n\n")

	b.WriteString("### Analytical\n")
	b.WriteString("Decompose before designing. No hand-waving.\n")
	b.WriteString("- List the entities first. Before creating tables, name ALL the real-world things being modeled.\n")
	b.WriteString("- Find the structure. Is this a tree (categories), a graph (friends), a queue (tasks), a state machine (orders)?\n")
	b.WriteString("- Trace the chain. User creates order → order has items → items reduce inventory → inventory triggers restock.\n")
	b.WriteString("- Do the math. If the prompt says 'thousands of users,' design indexes. If 'real-time,' consider computed fields.\n\n")

	b.WriteString("## Entity vs Action — DO NOT create tables for actions\n\n")
	b.WriteString("CRITICAL: Only create tables for THINGS THAT STORE DATA (nouns/entities). Never create tables for actions, features, or UI views.\n\n")
	b.WriteString("These are ENTITIES (create tables): users, products, orders, questions, topics, test_attempts, readiness_scores\n")
	b.WriteString("These are ACTIONS (use routes on existing tables, NOT new tables): signup, login, logout, register, signin, sign_up, log_in, forgot_password, reset_password, verify_email, checkout, payment, subscribe, unsubscribe, activate, deactivate, approve, reject, submit, publish, archive, import, export, sync, refresh, compute, calculate, generate, analyze\n")
	b.WriteString("These are VIEWS (computed endpoints, NOT tables): dashboard, overview, analytics, stats, reports, summary, leaderboard, feed, timeline, search, recommendations, study_map, performance, progress, history\n\n")
	b.WriteString("When the user mentions 'signup' or 'login', create routes that use the 'users' table — do NOT create a 'signups' or 'logins' table.\n")
	b.WriteString("When the user mentions 'dashboard' or 'analytics', create computed GET endpoints that query existing tables — do NOT create a 'dashboards' table.\n")
	b.WriteString("When the user mentions 'readiness' or 'mastery', create a scores/tracking table (readiness_scores, topic_mastery) — these are entities that store computed results.\n\n")

	b.WriteString("## State Machines\n\n")
	b.WriteString("When an entity has a LIFECYCLE with distinct states, define a state_machine on the schema.\n")
	b.WriteString("DO NOT use state machines for simple boolean flags (active/inactive). Use them for multi-step workflows.\n\n")
	b.WriteString("Examples:\n")
	b.WriteString("- Orders: draft → submitted → approved → shipped → delivered (with rejected branch)\n")
	b.WriteString("- Articles: draft → review → published → archived\n")
	b.WriteString("- Tickets: open → in_progress → resolved → closed\n")
	b.WriteString("- Applications: submitted → under_review → accepted/rejected\n\n")
	b.WriteString("Format — add state_machine to the schema object:\n")
	b.WriteString("\"state_machine\": {\"field\": \"status\", \"initial\": \"draft\", \"transitions\": [\n")
	b.WriteString("  {\"from\": \"draft\", \"to\": \"submitted\", \"action\": \"submit\", \"guard\": {\"condition\": \"total > 0\"}},\n")
	b.WriteString("  {\"from\": \"submitted\", \"to\": \"approved\", \"action\": \"approve\", \"guard\": {\"role\": \"admin\"}},\n")
	b.WriteString("  {\"from\": \"submitted\", \"to\": \"rejected\", \"action\": \"reject\", \"guard\": {\"role\": \"admin\"}}\n")
	b.WriteString("]}\n\n")
	b.WriteString("Guard types:\n")
	b.WriteString("- role: require specific user role (\"admin\", \"manager\", \"reviewer\")\n")
	b.WriteString("- condition: field condition on the row (\"total > 0\", \"items_count > 0\")\n\n")
	b.WriteString("IMPORTANT: Transition routes are AUTO-GENERATED from the state_machine definition.\n")
	b.WriteString("Do NOT manually create routes like POST /orders/:id/approve — they are created automatically.\n")
	b.WriteString("Just define the state_machine on the schema and VibeServe handles the rest.\n\n")

	b.WriteString("## Architectural Heuristics\n\n")
	b.WriteString("When designing an API, think beyond simple CRUD. For every request, consider:\n\n")
	b.WriteString("1. STATE TRANSITIONS: If an entity has a lifecycle (draft→active→closed),\n")
	b.WriteString("   create explicit action endpoints (POST /resource/:id/activate) instead\n")
	b.WriteString("   of generic PUT with a status field.\n\n")
	b.WriteString("2. COMPUTED ENDPOINTS: If users need aggregated/calculated data, create\n")
	b.WriteString("   dedicated endpoints with the computation in the script, not raw SELECTs.\n\n")
	b.WriteString("3. VALIDATION GUARDS: Add business rule checks before mutations. Check\n")
	b.WriteString("   time limits, prevent duplicates, verify prerequisites.\n\n")
	b.WriteString("4. LIFECYCLE HOOKS: When one action should trigger updates elsewhere,\n")
	b.WriteString("   include that logic. Submitting a quiz should update the student's stats.\n\n")
	b.WriteString("Your blueprint MUST include at least 2 routes that go beyond basic CRUD.\n\n")

	b.WriteString("## Route Coverage — EVERY table MUST have routes\n\n")
	b.WriteString("CRITICAL: For EVERY table in the schema, you MUST generate routes to access it.\n")
	b.WriteString("Do NOT create tables without routes — that makes the table inaccessible.\n\n")
	b.WriteString("Minimum routes per table:\n")
	b.WriteString("- GET /{table} — list all (with pagination)\n")
	b.WriteString("- GET /{table}/:id — get by ID\n")
	b.WriteString("- POST /{table} — create\n")
	b.WriteString("- PUT /{table}/:id — update\n")
	b.WriteString("- DELETE /{table}/:id — soft delete\n\n")
	b.WriteString("For the 'users' table specifically, ALWAYS include:\n")
	b.WriteString("- GET /users/me — get current user profile (from auth token)\n")
	b.WriteString("- PUT /users/me — update current user profile\n")
	b.WriteString("- GET /admin/users — admin list all users\n")
	b.WriteString("- PUT /admin/users/:id — admin update user\n\n")
	b.WriteString("Routes can be namespaced (e.g. /admin/questions, /student/tests) but the underlying table MUST be accessible.\n")

	return b.String()
}

// BuildPlanPrompt creates a prompt that asks the LLM to break a request into steps.
func BuildPlanPrompt(userRequest string) string {
	return fmt.Sprintf(`The user wants: "%s"

You are a senior backend architect. Design a COMPLETE, professional API — not a toy CRUD app.

## CRITICAL: Entity vs Action — only create tables for data entities

NEVER create tables for actions or UI views. Only create tables for THINGS THAT STORE DATA.
- signup, login, logout, register, checkout, payment → these are ROUTES on the users/orders table, NOT separate tables
- dashboard, analytics, stats, reports, overview, leaderboard → these are COMPUTED ENDPOINTS that query existing tables, NOT tables
- readiness, mastery → these ARE tables (readiness_scores, topic_mastery) because they store computed results over time

## Step 1: Domain Decomposition (do this BEFORE writing steps)

Fully analyze the user's request. Identify:
- EVERY data entity mentioned or implied (things that store rows of data)
- Distinguish entities (nouns that store data) from actions (verbs) and views (computed reads)
- ALL relationships between entities (1:1, 1:N, N:M with join tables)
- ALL columns for each entity — be exhaustive. Include type fields, status fields, metadata, foreign keys
- Entity lifecycles that need STATE MACHINES (orders: draft→submitted→approved, tickets: open→resolved→closed)
- Entity lifecycles (status transitions like draft→active→completed)
- Computed data (aggregations, analytics, dashboards, rankings, progress tracking)
- Business rules and validation guards
- Features the user described but didn't name as tables (e.g., "readiness" = a readiness_scores table)

DO NOT skip entities or columns that the user explicitly described. If the user says "questions need to include question type, difficulty, options, explanation for each option" — the schema MUST have question_type, difficulty, options, explanation columns from step 1.

## Step 2: Write the plan

Break into 5-12 implementation steps depending on complexity. Simple apps need 5, complex domain apps need 8-12.

Rules:
- Step 1: Design ALL tables with ALL columns, proper relationships (foreign keys, join tables). Be exhaustive — every field the user mentioned MUST appear here. Name every column explicitly.
- Middle steps: Group by DOMAIN MODULE, not HTTP verb. Each step implements one business capability.
- Entities with lifecycles MUST include a state_machine definition with transitions and guards — do NOT just use a status TEXT column
- At least 3 steps MUST include non-CRUD routes: state transitions, computed endpoints, analytics, or validation guards.
- Include steps for EVERY feature the user described — readiness tracking, analytics, study maps, dashboards, etc. Don't skip features.
- Final step: Seed data that exercises the business logic (multiple user roles, various states, enough data for analytics to be meaningful).
- Each step description must be SPECIFIC — name tables, columns, routes, and business logic.

## Output

Output a JSON object with two keys:
1. "steps" — array of step description strings (the implementation plan)
2. "schemas" — array of table definitions for the ER diagram preview

The schemas array uses this format:
{"table": "tablename", "columns": [{"name": "col", "type": "INTEGER|TEXT|REAL|BOOLEAN|DATETIME", "primary": true, "auto": true, "required": true, "unique": true, "references": "other_table.id"}]}

ONLY include data entity tables in schemas — NOT actions (login, signup) or views (dashboard, analytics).

Output ONLY the JSON object, no markdown fences, no explanation.

Example output structure:
{"steps": ["Create tables: users (...), subjects (...), topics (...)", "Implement auth routes...", "Add analytics..."], "schemas": [{"table": "users", "columns": [{"name": "id", "type": "INTEGER", "primary": true, "auto": true}, {"name": "email", "type": "TEXT", "required": true, "unique": true}, {"name": "role", "type": "TEXT", "required": true}]}, {"table": "topics", "columns": [{"name": "id", "type": "INTEGER", "primary": true, "auto": true}, {"name": "subject_id", "type": "INTEGER", "required": true, "references": "subjects.id"}, {"name": "name", "type": "TEXT", "required": true}]}]}`, userRequest)
}

// PlanResult holds the parsed plan steps and optional schema preview.
type PlanResult struct {
	Steps   []string
	Schemas []manifest.Schema
}

// ExtractPlan parses LLM plan output. Handles three formats:
// 1. {"steps": [...], "schemas": [...]} — structured plan with schema preview
// 2. ["step1", "step2"] — plain array of steps
// 3. [{"step": "step1"}, ...] — array of step objects
func ExtractPlan(raw string) ([]string, error) {
	result, err := ExtractPlanWithSchemas(raw)
	if err != nil {
		return nil, err
	}
	return result.Steps, nil
}

// ExtractPlanWithSchemas parses LLM plan output and returns both steps and schemas.
func ExtractPlanWithSchemas(raw string) (*PlanResult, error) {
	raw = strings.TrimSpace(raw)

	// Try structured format first: {"steps": [...], "schemas": [...]}
	if idx := strings.Index(raw, "{"); idx >= 0 {
		type structuredPlan struct {
			Steps   []string          `json:"steps"`
			Schemas []manifest.Schema `json:"schemas"`
		}
		var sp structuredPlan
		// Find the outermost { }
		end := strings.LastIndex(raw, "}")
		if end > idx {
			if err := json.Unmarshal([]byte(raw[idx:end+1]), &sp); err == nil && len(sp.Steps) > 0 {
				return &PlanResult{Steps: sp.Steps, Schemas: sp.Schemas}, nil
			}
		}
	}

	// Fallback to legacy array format
	steps, err := extractPlanArray(raw)
	if err != nil {
		return nil, err
	}
	return &PlanResult{Steps: steps}, nil
}

// extractPlanArray parses a JSON array of step descriptions from LLM output.
func extractPlanArray(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)

	// Find the array
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in plan response")
	}

	arrayJSON := []byte(raw[start : end+1])

	// Try as string array first
	var steps []string
	if err := json.Unmarshal(arrayJSON, &steps); err == nil && len(steps) > 0 {
		return steps, nil
	}

	// Try as array of objects — extract first string value from each
	var objects []map[string]any
	if err := json.Unmarshal(arrayJSON, &objects); err == nil && len(objects) > 0 {
		var extracted []string
		for _, obj := range objects {
			// Try common keys: step, description, name, title, text
			for _, key := range []string{"step", "description", "name", "title", "text"} {
				if v, ok := obj[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						extracted = append(extracted, s)
						break
					}
				}
			}
			// Fallback: use the first string value found
			if len(extracted) < len(objects) {
				for _, v := range obj {
					if s, ok := v.(string); ok && s != "" {
						extracted = append(extracted, s)
						break
					}
				}
			}
		}
		if len(extracted) > 0 {
			return extracted, nil
		}
	}

	return nil, fmt.Errorf("no JSON array found in plan response")
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
