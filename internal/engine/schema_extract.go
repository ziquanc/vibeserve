package engine

import (
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// tablePattern matches "tablename (col1, col2, col3 FK, ...)" in plan step text.
// Captures: table name, column list inside parens.
var tablePattern = regexp.MustCompile(`\b([a-z][a-z0-9_]*)\s*\(([^)]{3,})\)`)

// fkPattern matches "xxx_id FK" or "xxx_id" column names that look like foreign keys.
var fkPattern = regexp.MustCompile(`^([a-z_]+)_id$`)

// extractSchemasFromSteps parses plan step descriptions to extract table names
// and column definitions for building an ER diagram preview.
// This is a best-effort parser — it won't catch everything, but handles the
// common patterns the LLM produces in plan steps.
func extractSchemasFromSteps(steps []string) []manifest.Schema {
	seen := make(map[string]bool)
	var schemas []manifest.Schema

	// Common words that look like table names but aren't
	skipWords := map[string]bool{
		"e": true, "g": true, "eg": true, "i": true,
		"json": true, "text": true, "integer": true, "boolean": true,
		"real": true, "date": true, "datetime": true, "null": true,
		"true": true, "false": true, "status": true, "type": true,
		"values": true, "where": true, "select": true, "from": true,
		"insert": true, "update": true, "delete": true, "create": true,
		"filter": true, "example": true, "default": true, "like": true,
	}

	fullText := strings.Join(steps, " ")
	matches := tablePattern.FindAllStringSubmatch(fullText, -1)

	// Pass 1: collect all table names first so FK detection can link to ANY table.
	allTableNames := make(map[string]bool)
	type tableRaw struct {
		name    string
		colText string
	}
	var rawTables []tableRaw

	for _, match := range matches {
		tableName := strings.ToLower(match[1])
		colText := match[2]

		if skipWords[tableName] {
			continue
		}
		if seen[tableName] {
			continue
		}
		seen[tableName] = true
		allTableNames[tableName] = true
		rawTables = append(rawTables, tableRaw{name: tableName, colText: colText})
	}

	// Pass 2: parse columns with full table name knowledge for FK detection.
	for _, rt := range rawTables {
		columns := parseColumnsFromText(rt.colText, allTableNames)
		if len(columns) == 0 {
			continue
		}

		schemas = append(schemas, manifest.Schema{
			Table:   rt.name,
			Columns: columns,
		})
	}

	return schemas
}

// parseColumnsFromText parses "id, name, email, role: admin/student, user_id FK"
// into manifest.Column slice. allTables contains ALL known table names for FK detection.
func parseColumnsFromText(text string, allTables map[string]bool) []manifest.Column {
	// Build extended lookup with singular forms too
	tableNames := make(map[string]bool)
	for name := range allTables {
		tableNames[name] = true
		// Also register singular form
		if strings.HasSuffix(name, "s") {
			tableNames[name[:len(name)-1]] = true
		}
	}

	parts := strings.Split(text, ",")
	var columns []manifest.Column

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Extract just the column name (first word before : or space with type info)
		// "role: admin/student" → "role"
		// "exam_type: SPM/STPM/UEC" → "exam_type"
		// "id" → "id"
		// "user_id FK" → "user_id"
		name := part
		if idx := strings.IndexAny(name, ": "); idx > 0 {
			name = name[:idx]
		}
		name = strings.TrimSpace(name)
		name = strings.ToLower(name)

		// Skip non-column words
		if name == "" || name == "fk" || name == "pk" || name == "and" || name == "or" || name == "with" {
			continue
		}
		// Skip if it looks like a description, not a column name
		if strings.Contains(name, "'") || strings.Contains(name, "\"") {
			continue
		}
		// Only allow valid identifier chars
		if !validColumnName.MatchString(name) {
			continue
		}

		col := manifest.Column{
			Name: name,
			Type: guessColumnType(name, part),
		}

		// Detect primary key
		if name == "id" {
			col.Primary = true
			col.Auto = true
		}

		// Detect foreign key from _id suffix
		if fkMatch := fkPattern.FindStringSubmatch(name); fkMatch != nil {
			refResource := fkMatch[1]
			// Try plural form first
			refTable := refResource + "s"
			if tableNames[refTable] || tableNames[refResource] {
				if tableNames[refTable] {
					col.References = refTable + ".id"
				} else {
					col.References = refResource + ".id"
				}
			}
		}

		// Detect FK from "FK" in the text
		if strings.Contains(strings.ToUpper(part), "FK") && col.References == "" {
			if fkMatch := fkPattern.FindStringSubmatch(name); fkMatch != nil {
				col.References = fkMatch[1] + "s.id"
			}
		}

		columns = append(columns, col)
	}

	return columns
}

// guessColumnType infers a column type from name and context.
func guessColumnType(name, context string) string {
	lower := strings.ToLower(name)
	ctx := strings.ToLower(context)

	if lower == "id" || strings.HasSuffix(lower, "_id") {
		return "INTEGER"
	}
	if strings.Contains(lower, "price") || strings.Contains(lower, "amount") ||
		strings.Contains(lower, "score") || strings.Contains(lower, "pct") ||
		strings.Contains(lower, "percentage") || strings.Contains(lower, "rate") {
		return "REAL"
	}
	if strings.Contains(lower, "is_") || strings.Contains(lower, "has_") ||
		strings.Contains(lower, "active") || strings.Contains(lower, "enabled") ||
		strings.Contains(ctx, "boolean") {
		return "BOOLEAN"
	}
	if strings.HasSuffix(lower, "_at") || strings.Contains(lower, "date") ||
		strings.Contains(lower, "time") {
		return "DATETIME"
	}
	if strings.Contains(lower, "count") || strings.Contains(lower, "total") ||
		strings.Contains(lower, "marks") || strings.Contains(lower, "minutes") ||
		strings.Contains(lower, "seconds") || strings.Contains(lower, "order") {
		return "INTEGER"
	}
	return "TEXT"
}
