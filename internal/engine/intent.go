package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// IntentType represents what the proxy should do with a request.
type IntentType int

const (
	// IntentCreateTable means we need a brand new table + CRUD routes.
	IntentCreateTable IntentType = iota

	// IntentAddRoute means the target table exists and has all needed columns.
	// We just need a new route path mapping to it.
	IntentAddRoute

	// IntentAddColumn means the target table exists but the body has new fields.
	IntentAddColumn

	// IntentAddRelationshipRoute means both tables exist, just need a new route.
	IntentAddRelationshipRoute

	// IntentAI means the request is ambiguous and needs AI interpretation.
	IntentAI
)

func (t IntentType) String() string {
	switch t {
	case IntentCreateTable:
		return "CreateTable"
	case IntentAddRoute:
		return "AddRoute"
	case IntentAddColumn:
		return "AddColumn"
	case IntentAddRelationshipRoute:
		return "AddRelationshipRoute"
	case IntentAI:
		return "AI"
	default:
		return "Unknown"
	}
}

// Intent represents the analyzed intent of an HTTP request.
type Intent struct {
	Type IntentType

	// For IntentCreateTable:
	TableName string
	Columns   []manifest.Column

	// For IntentAddRoute / IntentAddColumn:
	TargetTable string
	NewRoute    manifest.Route
	NewScript   manifest.Script

	// For IntentAddColumn:
	ColumnsToAdd []manifest.Column

	// For IntentAddRelationshipRoute:
	ParentTable string
	ChildTable  string

	// For IntentAI:
	Reason string

	// Common:
	Prompt   string // prompt to send to AI if IntentAI
	Resource string // extracted resource name
}

// IntentAnalyzer examines HTTP requests against the current schema
// and determines what action to take.
type IntentAnalyzer struct {
	graph    *SchemaGraph
	manifest *manifest.Manifest
}

// NewIntentAnalyzer creates an IntentAnalyzer from the current manifest.
func NewIntentAnalyzer(m *manifest.Manifest) *IntentAnalyzer {
	return &IntentAnalyzer{
		graph:    NewSchemaGraph(m),
		manifest: m,
	}
}

// Analyze determines what to do with an incoming HTTP request.
//
// Decision tree:
//  1. Relationship route (/a/:id/b where both tables exist) -> IntentAddRelationshipRoute
//  2. Resource maps to existing table, no new fields -> IntentAddRoute
//  3. Resource maps to existing table, new fields in body -> IntentAddColumn
//  4. Resource does NOT map to existing table -> IntentCreateTable
//  5. Ambiguous path (dashboard, stats, etc.) -> IntentAI
func (ia *IntentAnalyzer) Analyze(method, path string, body map[string]any, queryParams map[string]string) *Intent {
	resource := ia.extractResource(path)
	intent := &Intent{Resource: resource}

	// Step 1: Check for relationship route
	if parent, _, child, ok := ia.graph.IsRelationshipRoute(path); ok {
		childInfo := ia.graph.FindTableByResource(child)
		if childInfo != nil {
			intent.Type = IntentAddRelationshipRoute
			intent.ParentTable = parent
			intent.ChildTable = childInfo.Name
			return intent
		}
	}

	// Step 2: Check for ambiguous paths that should go straight to AI
	if ia.isAmbiguousPath(path, resource) {
		intent.Type = IntentAI
		intent.Reason = fmt.Sprintf("path %q looks like an action or aggregate, not a resource", path)
		intent.Prompt = ia.BuildSmartPrompt(method, path, body, queryParams)
		return intent
	}

	// Step 3: Try to match resource to existing table
	tableInfo := ia.graph.FindTableByResource(resource)

	if tableInfo != nil {
		// Table exists. Check if body has new fields.
		intent.TargetTable = tableInfo.Name

		if len(body) > 0 {
			missing := ia.graph.GetMissingColumns(tableInfo.Name, body)
			if len(missing) > 0 {
				// New fields detected -> need to add columns
				intent.Type = IntentAddColumn
				intent.ColumnsToAdd = make([]manifest.Column, 0, len(missing))
				for _, mc := range missing {
					col := manifest.Column{
						Name: mc.Name,
						Type: mc.Type,
					}
					// Check if this field is a FK
					if fk := ia.graph.DetectFKFromField(mc.Name, mc.Type); fk != nil {
						col.References = fmt.Sprintf("%s.%s", fk.RefTable, fk.RefColumn)
					}
					intent.ColumnsToAdd = append(intent.ColumnsToAdd, col)
				}
				return intent
			}
		}

		// Table exists, no new fields -> just add a route
		intent.Type = IntentAddRoute
		return intent
	}

	// Step 4: Table does NOT exist -> create it
	intent.Type = IntentCreateTable
	intent.TableName = ia.guessTableName(resource)

	// Build columns from body
	intent.Columns = ia.buildColumnsFromBody(body)

	return intent
}

// BuildSmartPrompt creates a detailed prompt for the AI when we can't
// determine intent deterministically.
func (ia *IntentAnalyzer) BuildSmartPrompt(method, path string, body map[string]any, queryParams map[string]string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("A user sent an HTTP request to a VibeServe API:\n\n"))
	b.WriteString(fmt.Sprintf("%s %s\n", method, path))

	if len(body) > 0 {
		bodyJSON, _ := json.MarshalIndent(body, "", "  ")
		b.WriteString(fmt.Sprintf("Body:\n%s\n", string(bodyJSON)))
	}
	if len(queryParams) > 0 {
		b.WriteString("Query parameters:\n")
		for k, v := range queryParams {
			b.WriteString(fmt.Sprintf("  %s=%s\n", k, v))
		}
	}

	b.WriteString("\n---\n\n")
	b.WriteString("Current API state:\n\n")
	b.WriteString(ia.graph.BuildSchemaSummary())

	b.WriteString("\n---\n\n")
	b.WriteString("Analyze this request and determine what API changes are needed:\n")
	b.WriteString("- If the path maps to an existing resource, create a route that uses that resource's table\n")
	b.WriteString("- If the body contains new fields for an existing table, add those columns\n")
	b.WriteString("- If the body suggests a new entity that should be its own table, create it with proper foreign keys\n")
	b.WriteString("- If multiple related entities are detected, create all necessary tables and link them with foreign keys\n")
	b.WriteString("- Detect foreign keys: if a field like 'product_id' references an existing 'products' table, set references: \"products.id\"\n")
	b.WriteString("- Preserve ALL existing tables, routes, scripts, and seeds — only ADD new ones\n")
	b.WriteString("- Include an auto-increment id primary key column and created_at DATETIME column for new tables\n")
	b.WriteString("- Generate full CRUD routes for any new table (GET list, GET by id, POST create, PUT update, DELETE)\n")
	b.WriteString("\nOutput the COMPLETE updated manifest as JSON. No markdown fences, no explanation.\n")

	return b.String()
}

// extractResource gets the most likely resource name from a URL path.
// e.g., "/smart/product" -> "product", "/api/v2/users" -> "users",
// "/products/1/orders" -> "orders" (last non-param segment).
func (ia *IntentAnalyzer) extractResource(path string) string {
	segments := splitPath(path)
	// Walk backwards to find the last non-param segment
	for i := len(segments) - 1; i >= 0; i-- {
		s := segments[i]
		if strings.HasPrefix(s, ":") {
			continue
		}
		// Skip common prefixes
		lower := strings.ToLower(s)
		if lower == "api" || lower == "v1" || lower == "v2" || lower == "v3" {
			continue
		}
		return s
	}
	return ""
}

// isAmbiguousPath returns true for paths that look like actions or aggregates,
// not resources. These should go to AI.
func (ia *IntentAnalyzer) isAmbiguousPath(path, resource string) bool {
	// No resource extracted
	if resource == "" {
		return true
	}

	// Common non-resource path segments
	ambiguous := []string{
		"dashboard", "stats", "analytics", "search", "auth", "login",
		"logout", "register", "settings", "config", "admin", "health",
		"status", "metrics", "reports", "export", "import", "upload",
		"download", "notify", "webhook", "callback", "oauth", "token",
	}
	lower := strings.ToLower(resource)
	for _, a := range ambiguous {
		if lower == a {
			return true
		}
	}

	return false
}

// guessTableName derives a table name from a resource.
// Prefers plural form.
func (ia *IntentAnalyzer) guessTableName(resource string) string {
	lower := strings.ToLower(resource)
	// If it's already plural-ish, use as-is
	if strings.HasSuffix(lower, "s") {
		return lower
	}
	return pluralize(lower)
}

// buildColumnsFromBody creates column definitions from a request body.
// Adds id (PK auto) and created_at as defaults.
// Detects foreign keys from field_name patterns.
func (ia *IntentAnalyzer) buildColumnsFromBody(body map[string]any) []manifest.Column {
	columns := []manifest.Column{
		{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
	}

	for k, v := range body {
		col := manifest.Column{
			Name: k,
			Type: guessType(v),
		}

		// Detect FK
		if fk := ia.graph.DetectFKFromField(k, col.Type); fk != nil {
			col.References = fmt.Sprintf("%s.%s", fk.RefTable, fk.RefColumn)
		}

		columns = append(columns, col)
	}

	columns = append(columns, manifest.Column{
		Name:    "created_at",
		Type:    "DATETIME",
		Default: "NOW",
	})

	return columns
}
