package manifest

// timestampColumns defines the standard timestamp columns injected into every table.
var timestampColumns = []Column{
	{Name: "created_at", Type: "DATETIME", Default: "NOW"},
	{Name: "updated_at", Type: "DATETIME", Default: "NOW"},
	{Name: "deleted_at", Type: "DATETIME"},
}

// InjectTimestamps returns a copy of the schema with created_at, updated_at,
// and deleted_at columns appended. Columns that already exist are skipped.
// The original schema is not mutated.
func InjectTimestamps(schema Schema) Schema {
	existing := make(map[string]bool, len(schema.Columns))
	for _, c := range schema.Columns {
		existing[c.Name] = true
	}

	cols := make([]Column, len(schema.Columns))
	copy(cols, schema.Columns)

	for _, ts := range timestampColumns {
		if !existing[ts.Name] {
			cols = append(cols, ts)
		}
	}

	return Schema{
		Table:   schema.Table,
		Columns: cols,
	}
}
