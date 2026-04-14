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
        {"name": "password_hash", "type": "TEXT", "required": true},
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
- crypto.hash_password(str) → bcrypt hash string
- crypto.verify_password(plain, hash) → true/false

### auth — Token generation (available when users table has password_hash)
- auth.generate_tokens(user_id, role, email) → {access_token: string, refresh_token: string}

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

	b.WriteString("## Design Checklist (run through ALL of these for every API design)\n\n")

	b.WriteString("### 1. SCHEMA: What stores data?\n")
	b.WriteString("- ENTITIES store data (create tables): users, products, orders, questions, topics\n")
	b.WriteString("- ACTIONS are routes, NOT tables: signup, login, checkout, approve, submit, publish\n")
	b.WriteString("- VIEWS are computed endpoints, NOT tables: dashboard, analytics, stats, reports\n")
	b.WriteString("- TRACKING tables store computed results: topic_mastery, readiness_scores, activity_logs\n\n")

	b.WriteString("### 2. AUTH (auto-generated)\n")
	b.WriteString("When creating a users table, include a password_hash TEXT column. Auth routes are AUTO-GENERATED:\n")
	b.WriteString("- POST /auth/register, POST /auth/login, POST /auth/refresh, POST /auth/logout\n")
	b.WriteString("- GET /me, PUT /me (user profile)\n")
	b.WriteString("- POST /auth/forgot-password, POST /auth/reset-password, POST /auth/verify-email\n")
	b.WriteString("Do NOT manually create auth routes — they are auto-generated when password_hash column exists.\n")
	b.WriteString("In scripts, use request.auth() to check authentication — returns {user_id, role, email} or undefined.\n")
	b.WriteString("Use auth.generate_tokens(user_id, role, email) to create JWT + refresh tokens.\n")
	b.WriteString("Use crypto.hash_password(plain) and crypto.verify_password(plain, hash) for passwords.\n\n")

	b.WriteString("### 3. RELATIONSHIPS: How do tables connect?\n")
	b.WriteString("- Main entities: full tables with all columns\n")
	b.WriteString("- Junction tables (N:M): post_tags, user_roles — managed through parent routes\n")
	b.WriteString("- Child records: test_answers, order_items — managed through parent routes\n")
	b.WriteString("- Foreign keys: every _id column references a parent table\n\n")

	b.WriteString("### 4. STATE MACHINES: What has a lifecycle?\n")
	b.WriteString("- Multi-step workflows need state_machine: orders (draft→submitted→approved), articles (draft→published)\n")
	b.WriteString("- Simple flags do NOT: active/inactive is just a boolean column\n")
	b.WriteString("- Format: {\"state_machine\": {\"field\": \"status\", \"initial\": \"draft\", \"transitions\": [{\"from\": \"draft\", \"to\": \"submitted\", \"action\": \"submit\", \"guard\": {\"role\": \"admin\"}}]}}\n")
	b.WriteString("- Guard types: role (\"admin\") and/or condition (\"total > 0\")\n")
	b.WriteString("- Transition routes (POST /table/:id/action) are AUTO-GENERATED — do NOT create them manually\n\n")

	b.WriteString("### 5. ROUTES: Who uses the API and what do they need?\n")
	b.WriteString("For EACH user role, design complete route coverage:\n\n")
	b.WriteString("PUBLIC (no auth):\n")
	b.WriteString("- Read-only access to public content: GET /posts, GET /products\n")
	b.WriteString("- Auth flows: POST /auth/register, POST /auth/login\n\n")
	b.WriteString("AUTHENTICATED USER:\n")
	b.WriteString("- Profile: GET /me, PUT /me\n")
	b.WriteString("- Own resources: GET /my/orders, POST /my/posts\n")
	b.WriteString("- Actions: POST /tests/:id/submit, POST /orders/:id/cancel\n")
	b.WriteString("- Personal views: GET /my/dashboard, GET /my/analytics\n\n")
	b.WriteString("ADMIN:\n")
	b.WriteString("- Manage ALL main entities: /admin/users, /admin/orders, /admin/content\n")
	b.WriteString("- User management: list, view, activate/deactivate users\n")
	b.WriteString("- Platform analytics: GET /admin/dashboard, GET /admin/analytics\n")
	b.WriteString("- Content management: CRUD on all content tables\n\n")
	b.WriteString("RELATIONSHIPS:\n")
	b.WriteString("- GET /users/:id/posts — posts by a specific user\n")
	b.WriteString("- GET /posts/:id/comments — comments on a post\n")
	b.WriteString("- POST /posts/:id/tags — manage tags on a post (junction table)\n\n")

	b.WriteString("### 6. ROUTE TYPES: Not everything is CRUD\n")
	b.WriteString("Main entities → full CRUD (list, get, create, update, delete)\n")
	b.WriteString("Junction tables → managed via parent (POST /posts/:id/tags, DELETE /posts/:id/tags/:tagId)\n")
	b.WriteString("Child records → managed via parent (POST /tests/:id/answer)\n")
	b.WriteString("Computed data → read-only endpoints (GET /analytics/overview)\n")
	b.WriteString("Actions → POST with business logic (POST /orders/:id/approve)\n")
	b.WriteString("Dashboards → aggregated queries (GET /admin/dashboard, GET /student/dashboard)\n\n")

	b.WriteString("### 7. BUSINESS LOGIC: What rules exist?\n")
	b.WriteString("- Validation guards: check prerequisites before mutations (total > 0 before submit)\n")
	b.WriteString("- Computed endpoints: aggregations, statistics, progress tracking\n")
	b.WriteString("- Side effects: submitting a test updates mastery scores, placing an order reduces inventory\n")
	b.WriteString("- Access control: students see own data, admin sees all data\n\n")

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
