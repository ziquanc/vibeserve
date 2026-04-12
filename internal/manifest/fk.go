package manifest

import "strings"

// InferForeignKeys scans all schemas for columns matching {table}_id patterns
// and adds References if the referenced table exists and the column has no
// reference yet. This is a best-effort pass that catches obvious FKs
// missed during incremental schema building (e.g., proxy mode).
func InferForeignKeys(schemas []Schema) []Schema {
	// Build table lookup (lowercase name → original name)
	tables := make(map[string]string, len(schemas))
	for _, s := range schemas {
		tables[strings.ToLower(s.Table)] = s.Table
	}

	// Scan columns for _id patterns
	result := make([]Schema, len(schemas))
	for i, s := range schemas {
		cols := make([]Column, len(s.Columns))
		copy(cols, s.Columns)

		for j, col := range cols {
			// Skip if already has a reference
			if col.References != "" {
				continue
			}

			lower := strings.ToLower(col.Name)
			if !strings.HasSuffix(lower, "_id") {
				continue
			}

			// Extract resource: "user_id" → "user"
			resource := strings.TrimSuffix(lower, "_id")

			// Try plural form first (most common: user_id → users table)
			candidates := []string{
				resource + "s",    // user → users
				resource + "es",   // bus → buses
				resource,          // sheep → sheep
			}
			// Handle "ies" pluralization: category_id → categories
			if strings.HasSuffix(resource, "y") {
				candidates = append(candidates, resource[:len(resource)-1]+"ies")
			}

			for _, candidate := range candidates {
				if tableName, ok := tables[candidate]; ok {
					cols[j].References = tableName + ".id"
					break
				}
			}
		}

		result[i] = Schema{Table: s.Table, Columns: cols}
	}

	return result
}
