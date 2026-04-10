package engine

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// TableInfo holds summary information about a database table.
type TableInfo struct {
	Name        string
	ColumnCount int
}

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name       string
	Type       string
	Primary    bool
	Required   bool
	References string
}

// FKInfo describes a detected foreign key relationship.
type FKInfo struct {
	Column    string
	RefTable  string
	RefColumn string
}

// SchemaGraph provides lookup and analysis methods over the current manifest schema.
type SchemaGraph struct {
	tables      map[string]*manifest.Schema // lowercase table name -> schema
	tableLookup map[string]string           // singular/plural/variations -> table name
	manifest    *manifest.Manifest
}

// NewSchemaGraph builds a SchemaGraph from a manifest.
func NewSchemaGraph(m *manifest.Manifest) *SchemaGraph {
	sg := &SchemaGraph{
		tables:      make(map[string]*manifest.Schema),
		tableLookup: make(map[string]string),
		manifest:    m,
	}
	if m == nil {
		return sg
	}
	for i := range m.Schemas {
		s := &m.Schemas[i]
		name := strings.ToLower(s.Table)
		sg.tables[name] = s

		// Register variations: table name, singular, plural
		sg.tableLookup[name] = s.Table
		sg.tableLookup[singularize(name)] = s.Table
		sg.tableLookup[pluralize(name)] = s.Table
	}
	return sg
}

// HasTable returns true if the named table exists in the schema.
func (sg *SchemaGraph) HasTable(name string) bool {
	_, ok := sg.tables[strings.ToLower(name)]
	return ok
}

// FindTableByResource attempts to match a resource name (from a URL path) to an
// existing table. It tries the name as-is, singularized, and pluralized.
func (sg *SchemaGraph) FindTableByResource(resource string) *TableInfo {
	tableName, ok := sg.tableLookup[strings.ToLower(resource)]
	if !ok {
		return nil
	}
	s, ok := sg.tables[strings.ToLower(tableName)]
	if !ok {
		return nil
	}
	return &TableInfo{
		Name:        s.Table,
		ColumnCount: len(s.Columns),
	}
}

// DetectFKFromField checks whether a field name/type suggests a foreign key to
// an existing table. For example fieldName="user_id" with type "INTEGER" would
// match a "users" table.
func (sg *SchemaGraph) DetectFKFromField(fieldName, fieldType string) *FKInfo {
	lower := strings.ToLower(fieldName)

	// Pattern: <resource>_id
	if strings.HasSuffix(lower, "_id") {
		resource := strings.TrimSuffix(lower, "_id")
		if info := sg.FindTableByResource(resource); info != nil {
			return &FKInfo{
				Column:    fieldName,
				RefTable:  info.Name,
				RefColumn: "id",
			}
		}
	}

	// Pattern: <resource>Id (camelCase)
	if len(lower) > 2 {
		for i := 1; i < len(lower)-1; i++ {
			if lower[i] >= 'A' && lower[i] <= 'Z' {
				suffix := lower[i:]
				if strings.EqualFold(suffix, "Id") || strings.EqualFold(suffix, "ID") {
					resource := lower[:i]
					if info := sg.FindTableByResource(resource); info != nil {
						return &FKInfo{
							Column:    fieldName,
							RefTable:  info.Name,
							RefColumn: "id",
						}
					}
				}
			}
		}
	}

	return nil
}

// GetMissingColumns returns the body keys that don't have a corresponding column
// in the named table.
func (sg *SchemaGraph) GetMissingColumns(tableName string, body map[string]any) []ColumnInfo {
	s, ok := sg.tables[strings.ToLower(tableName)]
	if !ok {
		return nil
	}

	existing := make(map[string]bool, len(s.Columns))
	for _, c := range s.Columns {
		existing[strings.ToLower(c.Name)] = true
	}

	var missing []ColumnInfo
	for k, v := range body {
		if existing[strings.ToLower(k)] {
			continue
		}
		missing = append(missing, ColumnInfo{
			Name: k,
			Type: guessType(v),
		})
	}
	return missing
}

// IsRelationshipRoute parses a path like /products/:id/orders and returns
// the parent table, parent ID placeholder, child resource, and ok=true.
func (sg *SchemaGraph) IsRelationshipRoute(path string) (parent, parentID, child string, ok bool) {
	segments := splitPath(path)
	// Need at least 4 segments: /resource/:id/sub-resource
	if len(segments) < 4 {
		return "", "", "", false
	}

	// Walk segments looking for pattern: resource :id sub-resource
	for i := 0; i+2 < len(segments); i++ {
		if !strings.HasPrefix(segments[i+1], ":") {
			continue
		}
		parentInfo := sg.FindTableByResource(segments[i])
		if parentInfo == nil {
			continue
		}
		// Found a parent match; the child is the next segment after the ID
		if i+2 < len(segments) {
			child = segments[i+2]
			parent = parentInfo.Name
			parentID = segments[i+1]
			ok = true
			return
		}
	}
	return "", "", "", false
}

// BuildSchemaSummary returns a human-readable summary of the current schema state.
func (sg *SchemaGraph) BuildSchemaSummary() string {
	if sg.manifest == nil || len(sg.manifest.Schemas) == 0 {
		return "No tables defined yet."
	}
	var b strings.Builder
	b.WriteString("Tables:\n")
	for _, s := range sg.manifest.Schemas {
		b.WriteString(fmt.Sprintf("  %s (%d columns):\n", s.Table, len(s.Columns)))
		for _, c := range s.Columns {
			extra := ""
			if c.Primary {
				extra += " PRIMARY"
			}
			if c.Auto {
				extra += " AUTO"
			}
			if c.Required {
				extra += " NOT NULL"
			}
			if c.Unique {
				extra += " UNIQUE"
			}
			if c.References != "" {
				extra += fmt.Sprintf(" -> %s", c.References)
			}
			b.WriteString(fmt.Sprintf("    - %s %s%s\n", c.Name, c.Type, extra))
		}
	}
	if len(sg.manifest.Routes) > 0 {
		b.WriteString("Routes:\n")
		for _, r := range sg.manifest.Routes {
			b.WriteString(fmt.Sprintf("  %s %s\n", r.Method, r.Path))
		}
	}
	return b.String()
}

// singularize is a naive English singularizer.
func singularize(s string) string {
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "ses") && len(s) > 2 {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") && len(s) > 1 && s[len(s)-2] != 's' {
		return s[:len(s)-1]
	}
	return s
}

// pluralize is a naive English pluralizer.
func pluralize(s string) string {
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(rune(s[len(s)-2])) {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func isVowel(r rune) bool {
	return r == 'a' || r == 'e' || r == 'i' || r == 'o' || r == 'u'
}

// guessType infers a SQLite column type from a Go value.
func guessType(v any) string {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "INTEGER"
	case float32, float64:
		return "REAL"
	case bool:
		return "INTEGER" // SQLite stores bools as 0/1
	case string:
		return "TEXT"
	default:
		return "TEXT"
	}
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
