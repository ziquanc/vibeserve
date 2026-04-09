package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GoType maps a manifest Column to a Go type string.
func GoType(col manifest.Column) string {
	switch col.Type {
	case "INTEGER":
		return "int64"
	case "TEXT":
		return "string"
	case "REAL":
		return "float64"
	case "BOOLEAN":
		return "bool"
	case "DATE", "DATETIME":
		return "time.Time"
	default:
		return "any"
	}
}

// GenerateModels produces the Go source for internal/model/models.go.
func GenerateModels(schemas []manifest.Schema) string {
	var b strings.Builder

	needsTime := false
	for _, s := range schemas {
		for _, c := range s.Columns {
			if c.Type == "DATE" || c.Type == "DATETIME" {
				needsTime = true
			}
		}
	}

	b.WriteString("package model\n\n")
	if needsTime {
		b.WriteString("import \"time\"\n\n")
	}

	for i, s := range schemas {
		structName := TableToStructName(s.Table)
		b.WriteString(fmt.Sprintf("// %s represents a row in the %s table.\n", structName, s.Table))
		b.WriteString(fmt.Sprintf("type %s struct {\n", structName))
		for _, c := range s.Columns {
			fieldName := PascalCase(c.Name)
			goType := GoType(c)
			b.WriteString(fmt.Sprintf("\t%s %s `db:%q json:%q`\n", fieldName, goType, c.Name, c.Name))
		}
		b.WriteString("}\n")
		if i < len(schemas)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
