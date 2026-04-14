package watch

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateMigrationSQL produces PostgreSQL migration SQL from manifest changes.
// Returns empty string if no schema changes.
func GenerateMigrationSQL(changes []manifest.Change) string {
	var lines []string

	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeAddTable:
			if c.Schema != nil {
				lines = append(lines, generateCreateTableSQL(c.Schema))
			}
		case manifest.ChangeAddColumn:
			if c.Column != nil && c.Table != "" {
				lines = append(lines, generateAddColumnSQL(c.Table, c.Column))
			}
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n\n") + "\n"
}

func generateCreateTableSQL(schema *manifest.Schema) string {
	var colDefs []string
	for _, col := range schema.Columns {
		def := "  " + col.Name + " " + pgType(col)
		if col.Primary && col.Auto {
			def = "  " + col.Name + " SERIAL PRIMARY KEY"
		} else {
			if col.Primary {
				def += " PRIMARY KEY"
			}
			if col.Required && !col.Primary {
				def += " NOT NULL"
			}
			if col.Unique {
				def += " UNIQUE"
			}
			if col.Default != nil {
				switch v := col.Default.(type) {
				case string:
					upper := strings.ToUpper(v)
					if upper == "NOW" || strings.Contains(v, "(") {
						if upper == "NOW" {
							def += " DEFAULT NOW()"
						} else {
							def += " DEFAULT " + v
						}
					} else {
						def += fmt.Sprintf(" DEFAULT '%s'", strings.ReplaceAll(v, "'", "''"))
					}
				case bool:
					def += fmt.Sprintf(" DEFAULT %t", v)
				default:
					def += fmt.Sprintf(" DEFAULT %v", v)
				}
			}
			if col.References != "" {
				def += " REFERENCES " + col.References
			}
		}
		colDefs = append(colDefs, def)
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n%s\n);",
		schema.Table, strings.Join(colDefs, ",\n"))
}

func generateAddColumnSQL(table string, col *manifest.Column) string {
	def := col.Name + " " + pgType(*col)
	if col.Required {
		if col.Default != nil {
			def += " NOT NULL"
			switch v := col.Default.(type) {
			case string:
				upper := strings.ToUpper(v)
				if upper == "NOW" {
					def += " DEFAULT NOW()"
				} else {
					def += fmt.Sprintf(" DEFAULT '%s'", strings.ReplaceAll(v, "'", "''"))
				}
			default:
				def += fmt.Sprintf(" DEFAULT %v", v)
			}
		}
	}
	if col.Unique {
		def += " UNIQUE"
	}
	if col.References != "" {
		def += " REFERENCES " + col.References
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", table, def)
}

func pgType(col manifest.Column) string {
	if col.Primary && col.Auto {
		return "SERIAL"
	}
	switch col.Type {
	case "INTEGER":
		return "INTEGER"
	case "TEXT":
		return "TEXT"
	case "REAL":
		return "DOUBLE PRECISION"
	case "BOOLEAN":
		return "BOOLEAN"
	case "DATE":
		return "DATE"
	case "DATETIME":
		return "TIMESTAMP"
	default:
		return "TEXT"
	}
}
