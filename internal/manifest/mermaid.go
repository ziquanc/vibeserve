package manifest

import (
	"fmt"
	"strings"
)

// GenerateMermaidER produces a Mermaid erDiagram from manifest schemas.
func GenerateMermaidER(schemas []Schema) string {
	var b strings.Builder
	b.WriteString("erDiagram\n")

	// Collect relationships for drawing after entities.
	type relationship struct {
		from, to, label string
	}
	var rels []relationship

	for _, schema := range schemas {
		b.WriteString(fmt.Sprintf("    %s {\n", schema.Table))
		for _, col := range schema.Columns {
			// Skip timestamp columns for cleaner diagram.
			if col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
				continue
			}

			marker := ""
			if col.Primary {
				marker = " PK"
			} else if col.References != "" {
				marker = " FK"
			} else if col.Unique {
				marker = " UK"
			}

			// Show state machine states as a comment on the field.
			if schema.StateMachine != nil && col.Name == schema.StateMachine.Field {
				states := schema.StateMachine.ValidStates()
				marker += fmt.Sprintf(" \"%s\"", strings.Join(states, "|"))
			}

			b.WriteString(fmt.Sprintf("        %s %s%s\n", col.Type, col.Name, marker))

			// Collect relationship.
			if col.References != "" {
				refTable, _ := parseReference(col.References)
				if refTable != "" {
					rels = append(rels, relationship{
						from:  refTable,
						to:    schema.Table,
						label: col.Name,
					})
				}
			}
		}
		b.WriteString("    }\n")
	}

	// Draw relationships.
	for _, rel := range rels {
		b.WriteString(fmt.Sprintf("    %s ||--o{ %s : \"%s\"\n", rel.from, rel.to, rel.label))
	}

	return b.String()
}
