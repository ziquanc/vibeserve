package store

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// mapSQLiteType maps manifest column types to SQLite storage types.
func mapSQLiteType(manifestType string) string {
	switch strings.ToUpper(manifestType) {
	case "INTEGER":
		return "INTEGER"
	case "TEXT":
		return "TEXT"
	case "REAL":
		return "REAL"
	case "BOOLEAN":
		return "INTEGER" // stored as 0/1
	case "DATE":
		return "TEXT"
	case "DATETIME":
		return "TEXT"
	default:
		return "TEXT"
	}
}

// defaultValue renders a manifest column default value into SQL syntax.
func defaultValue(col manifest.Column) string {
	switch v := col.Default.(type) {
	case bool:
		if v {
			return "1"
		}
		return "0"
	case string:
		if v == "NOW" {
			return "CURRENT_TIMESTAMP"
		}
		return fmt.Sprintf("'%s'", v)
	case float64:
		// JSON numbers decode as float64; render as integer if no fractional part
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// BuildCreateTableSQL generates a CREATE TABLE IF NOT EXISTS statement from a manifest Schema.
func BuildCreateTableSQL(schema manifest.Schema) string {
	var cols []string
	for _, col := range schema.Columns {
		sqlType := mapSQLiteType(col.Type)
		var parts []string
		parts = append(parts, col.Name)
		parts = append(parts, sqlType)

		if col.Primary {
			if col.Auto {
				parts = append(parts, "PRIMARY KEY AUTOINCREMENT")
			} else {
				parts = append(parts, "PRIMARY KEY")
			}
		} else {
			if col.Required {
				parts = append(parts, "NOT NULL")
			}
			if col.Unique {
				parts = append(parts, "UNIQUE")
			}
			if col.Default != nil {
				parts = append(parts, "DEFAULT "+defaultValue(col))
			}
			if col.References != "" {
				parts = append(parts, "REFERENCES "+formatSQLReference(col.References))
			}
		}

		cols = append(cols, strings.Join(parts, " "))
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n)",
		schema.Table,
		strings.Join(cols, ",\n  "))
}

// BuildAddColumnSQL generates an ALTER TABLE ADD COLUMN statement.
func BuildAddColumnSQL(table string, col manifest.Column) string {
	sqlType := mapSQLiteType(col.Type)
	var parts []string
	parts = append(parts, col.Name)
	parts = append(parts, sqlType)
	if col.Required {
		if col.Default != nil {
			parts = append(parts, "NOT NULL")
			parts = append(parts, "DEFAULT "+defaultValue(col))
		}
	} else {
		if col.Default != nil {
			parts = append(parts, "DEFAULT "+defaultValue(col))
		}
	}
	if col.Unique {
		parts = append(parts, "UNIQUE")
	}
	if col.References != "" {
		parts = append(parts, "REFERENCES "+col.References)
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, strings.Join(parts, " "))
}

// formatSQLReference normalizes a reference to SQL format: table(column).
// Accepts "table.column" or "table(column)".
func formatSQLReference(ref string) string {
	// Already in SQL format
	if strings.Contains(ref, "(") {
		return ref
	}
	// Convert "table.column" to "table(column)"
	if idx := strings.IndexByte(ref, '.'); idx > 0 {
		return ref[:idx] + "(" + ref[idx+1:] + ")"
	}
	return ref
}
