package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateOpenAPI produces an OpenAPI 3.0.3 YAML document from a manifest.
func GenerateOpenAPI(m *manifest.Manifest) string {
	var b strings.Builder

	// Header
	b.WriteString("openapi: \"3.0.3\"\n")
	b.WriteString("info:\n")
	b.WriteString(fmt.Sprintf("  title: %q\n", m.Name))
	if m.Description != "" {
		b.WriteString(fmt.Sprintf("  description: %q\n", m.Description))
	}
	b.WriteString(fmt.Sprintf("  version: %q\n", m.Version))
	b.WriteString("\n")

	// Paths — group routes by converted path
	b.WriteString("paths:\n")

	// Group routes by path to emit them together
	type routeEntry struct {
		route manifest.Route
		oaPath string
	}
	// Preserve insertion order for paths
	seen := map[string]bool{}
	var orderedPaths []string
	pathRoutes := map[string][]manifest.Route{}
	for _, r := range m.Routes {
		oaPath := chiPath(r.Path)
		if !seen[oaPath] {
			seen[oaPath] = true
			orderedPaths = append(orderedPaths, oaPath)
		}
		pathRoutes[oaPath] = append(pathRoutes[oaPath], r)
	}

	for _, oaPath := range orderedPaths {
		b.WriteString(fmt.Sprintf("  %s:\n", oaPath))
		for _, route := range pathRoutes[oaPath] {
			method := strings.ToLower(route.Method)
			b.WriteString(fmt.Sprintf("    %s:\n", method))
			if route.Description != "" {
				b.WriteString(fmt.Sprintf("      summary: %q\n", route.Description))
			}
			b.WriteString(fmt.Sprintf("      operationId: %q\n", routeToMethodName(route)))

			// Path parameters
			params := extractPathParams(route.Path)
			if len(params) > 0 {
				b.WriteString("      parameters:\n")
				for _, param := range params {
					b.WriteString("        - name: " + param + "\n")
					b.WriteString("          in: path\n")
					b.WriteString("          required: true\n")
					b.WriteString("          schema:\n")
					b.WriteString("            type: integer\n")
					b.WriteString("            format: int64\n")
				}
			}

			// Request body
			if len(route.RequestBody) > 0 {
				b.WriteString("      requestBody:\n")
				b.WriteString("        required: true\n")
				b.WriteString("        content:\n")
				b.WriteString("          application/json:\n")
				b.WriteString("            schema:\n")
				b.WriteString("              type: object\n")
				b.WriteString("              properties:\n")
				for field, colType := range route.RequestBody {
					oapiType, oapiFormat := openAPIType(colType)
					b.WriteString(fmt.Sprintf("                %s:\n", field))
					b.WriteString(fmt.Sprintf("                  type: %s\n", oapiType))
					if oapiFormat != "" {
						b.WriteString(fmt.Sprintf("                  format: %s\n", oapiFormat))
					}
				}
			}

			// Responses
			b.WriteString("      responses:\n")
			statusCode := "200"
			if strings.ToUpper(route.Method) == "POST" {
				statusCode = "201"
			}
			b.WriteString(fmt.Sprintf("        \"%s\":\n", statusCode))
			b.WriteString("          description: Success\n")

			// Build schema ref from inferred table
			table := inferTableFromPath(route.Path)
			if table != "" {
				structName := TableToStructName(table)
				b.WriteString("          content:\n")
				b.WriteString("            application/json:\n")
				b.WriteString("              schema:\n")
				if route.ResponseType == "array" {
					b.WriteString("                type: array\n")
					b.WriteString("                items:\n")
					b.WriteString(fmt.Sprintf("                  $ref: \"#/components/schemas/%s\"\n", structName))
				} else {
					b.WriteString(fmt.Sprintf("                $ref: \"#/components/schemas/%s\"\n", structName))
				}
			}
		}
	}

	// Components / schemas
	if len(m.Schemas) > 0 {
		b.WriteString("\ncomponents:\n")
		b.WriteString("  schemas:\n")
		for _, schema := range m.Schemas {
			structName := TableToStructName(schema.Table)
			b.WriteString(fmt.Sprintf("    %s:\n", structName))
			b.WriteString("      type: object\n")
			if len(schema.Columns) > 0 {
				// Collect required fields
				var required []string
				for _, col := range schema.Columns {
					if col.Required && !col.Auto {
						required = append(required, col.Name)
					}
				}
				if len(required) > 0 {
					b.WriteString("      required:\n")
					for _, r := range required {
						b.WriteString(fmt.Sprintf("        - %s\n", r))
					}
				}
				b.WriteString("      properties:\n")
				for _, col := range schema.Columns {
					oapiType, oapiFormat := openAPIType(col.Type)
					b.WriteString(fmt.Sprintf("        %s:\n", col.Name))
					b.WriteString(fmt.Sprintf("          type: %s\n", oapiType))
					if oapiFormat != "" {
						b.WriteString(fmt.Sprintf("          format: %s\n", oapiFormat))
					}
				}
			}
		}
	}

	return b.String()
}

// openAPIType maps manifest column types to (OpenAPI type, format).
func openAPIType(colType string) (string, string) {
	switch strings.ToUpper(colType) {
	case "INTEGER", "INT":
		return "integer", "int64"
	case "TEXT", "VARCHAR", "STRING":
		return "string", ""
	case "REAL", "FLOAT", "DOUBLE":
		return "number", "double"
	case "BOOLEAN", "BOOL":
		return "boolean", ""
	case "DATE":
		return "string", "date"
	case "DATETIME", "TIMESTAMP":
		return "string", "date-time"
	default:
		return "string", ""
	}
}

// extractPathParams returns param names from a path like /vehicles/:id → ["id"].
func extractPathParams(path string) []string {
	segments := strings.Split(path, "/")
	var params []string
	for _, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			params = append(params, seg[1:])
		}
	}
	return params
}

// inferTableFromPath returns the first non-param path segment.
// e.g. /vehicles/:id → "vehicles", /bookings → "bookings".
func inferTableFromPath(path string) string {
	segments := strings.Split(path, "/")
	for _, seg := range segments {
		if seg == "" || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "{") {
			continue
		}
		return seg
	}
	return ""
}
