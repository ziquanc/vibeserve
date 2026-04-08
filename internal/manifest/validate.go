package manifest

import (
	"fmt"
	"strings"

	"github.com/d5/tengo/v2"
)

var validMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

var validColumnTypes = map[string]bool{
	"INTEGER": true, "TEXT": true, "REAL": true,
	"BOOLEAN": true, "DATE": true, "DATETIME": true,
}

// Validate runs all three validation layers on a manifest.
func Validate(m *Manifest) error {
	if err := validateStructural(m); err != nil {
		return fmt.Errorf("structural: %w", err)
	}
	if err := validateReferential(m); err != nil {
		return fmt.Errorf("referential: %w", err)
	}
	if err := validateCompilation(m); err != nil {
		return fmt.Errorf("compilation: %w", err)
	}
	return nil
}

func validateStructural(m *Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest version is required")
	}

	tableNames := make(map[string]bool)
	for _, s := range m.Schemas {
		if s.Table == "" {
			return fmt.Errorf("schema table name is required")
		}
		if tableNames[s.Table] {
			return fmt.Errorf("duplicate table name: %q", s.Table)
		}
		tableNames[s.Table] = true

		for _, c := range s.Columns {
			if c.Name == "" {
				return fmt.Errorf("column name is required in table %q", s.Table)
			}
			if !validColumnTypes[c.Type] {
				return fmt.Errorf("invalid column type %q for %s.%s", c.Type, s.Table, c.Name)
			}
		}
	}

	for _, r := range m.Routes {
		if r.Path == "" {
			return fmt.Errorf("route path is required")
		}
		if !validMethods[r.Method] {
			return fmt.Errorf("invalid HTTP method %q for route %s", r.Method, r.Path)
		}
	}

	scriptNames := make(map[string]bool)
	for _, s := range m.Scripts {
		if s.Name == "" {
			return fmt.Errorf("script name is required")
		}
		if scriptNames[s.Name] {
			return fmt.Errorf("duplicate script name: %q", s.Name)
		}
		scriptNames[s.Name] = true
	}

	return nil
}

func validateReferential(m *Manifest) error {
	tables := make(map[string]map[string]bool)
	for _, s := range m.Schemas {
		cols := make(map[string]bool)
		for _, c := range s.Columns {
			cols[c.Name] = true
		}
		tables[s.Table] = cols
	}

	scripts := make(map[string]bool)
	for _, s := range m.Scripts {
		scripts[s.Name] = true
	}

	for _, r := range m.Routes {
		if !scripts[r.Script] {
			return fmt.Errorf("route %s %s references nonexistent script %q", r.Method, r.Path, r.Script)
		}
	}

	for _, s := range m.Seeds {
		if _, ok := tables[s.Table]; !ok {
			return fmt.Errorf("seed references nonexistent table %q", s.Table)
		}
	}

	for _, s := range m.Schemas {
		for _, c := range s.Columns {
			if c.References == "" {
				continue
			}
			refTable, refCol := parseReference(c.References)
			if refTable == "" || refCol == "" {
				return fmt.Errorf("invalid reference format %q (expected table.column or table(column))", c.References)
			}
			cols, ok := tables[refTable]
			if !ok {
				return fmt.Errorf("%s.%s references nonexistent table %q", s.Table, c.Name, refTable)
			}
			if !cols[refCol] {
				return fmt.Errorf("%s.%s references nonexistent column %s.%s", s.Table, c.Name, refTable, refCol)
			}
		}
	}

	return nil
}

// parseReference extracts table and column from a reference string.
// Accepts both "table.column" and "table(column)" formats.
func parseReference(ref string) (table, column string) {
	// Try "table(column)" format first
	if idx := strings.IndexByte(ref, '('); idx > 0 {
		table = ref[:idx]
		rest := ref[idx+1:]
		if end := strings.IndexByte(rest, ')'); end > 0 {
			column = rest[:end]
			return table, column
		}
	}
	// Try "table.column" format
	if parts := strings.SplitN(ref, ".", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

// stdlibNames are the variable names injected into every Tengo script.
var stdlibNames = []string{"db", "request", "response", "date", "crypto", "log"}

func validateCompilation(m *Manifest) error {
	for _, s := range m.Scripts {
		script := tengo.NewScript([]byte(s.Code))
		for _, name := range stdlibNames {
			// Add empty maps so compiler knows these variables exist.
			_ = script.Add(name, map[string]interface{}{})
		}
		_, err := script.Compile()
		if err != nil {
			return fmt.Errorf("script %q: %w", s.Name, err)
		}
	}
	return nil
}
