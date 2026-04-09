package export

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Regexes for pattern matching Tengo script lines.
var (
	reRequestParam   = regexp.MustCompile(`^(\w+)\s*:=\s*request\.param\("(\w+)"\)`)
	reRequestQuery   = regexp.MustCompile(`^(\w+)\s*:=\s*request\.query\("(\w+)"\)`)
	reRequestBody    = regexp.MustCompile(`^(\w+)\s*:=\s*request\.body\(\)`)
	reDBQueryAssign  = regexp.MustCompile(`^(\w+)\s*:=\s*db\.query\("([^"]+)",\s*\[([^\]]*)\]\)`)
	reDBQueryOneAssign = regexp.MustCompile(`^(\w+)\s*:=\s*db\.query_one\("([^"]+)",\s*\[([^\]]*)\]\)`)
	reDBInsertAssign = regexp.MustCompile(`^(\w+)\s*:=\s*db\.insert\("(\w+)",\s*(.+)\)`)
	reDBInsertStmt   = regexp.MustCompile(`^db\.insert\("(\w+)",\s*(.+)\)`)
	reDBUpdateAssign = regexp.MustCompile(`^(\w+)\s*:=\s*db\.update\("(\w+)",\s*(\w+),\s*(.+)\)`)
	reDBUpdateStmt   = regexp.MustCompile(`^db\.update\("(\w+)",\s*(\w+),\s*(.+)\)`)
	reDBDeleteAssign = regexp.MustCompile(`^(\w+)\s*:=\s*db\.delete\("(\w+)",\s*(\w+)\)`)
	reDBDeleteStmt   = regexp.MustCompile(`^db\.delete\("(\w+)",\s*(\w+)\)`)
	reResponseJSON   = regexp.MustCompile(`^response\.json\((\w+)(?:,\s*(\d+))?\)`)
	reResponseFail   = regexp.MustCompile(`^response\.fail\((\d+),\s*"([^"]*)"\)`)
	reIfUndefined    = regexp.MustCompile(`^if\s+(\w+)\s*==\s*undefined\s*\{`)
	reElseUndefined  = regexp.MustCompile(`^\}\s*else\s*\{`)
	reCloseBrace     = regexp.MustCompile(`^\}$`)
	reSQLFrom        = regexp.MustCompile(`(?i)FROM\s+(\w+)`)
	reUnknownAssign  = regexp.MustCompile(`^(\w+)\s*:=\s*(.+)`)
	reComplexPkg     = regexp.MustCompile(`\b(date|crypto|math|fmt|os|io|strings|bytes)\.\w+`)
)

// PatternMatcher translates Tengo script lines to Go handler code line by line.
type PatternMatcher struct {
	// schemaMap maps table name -> Schema for struct name resolution.
	schemaMap map[string]manifest.Schema
}

// NewPatternMatcher creates a PatternMatcher with optional schema context.
func NewPatternMatcher(schemas []manifest.Schema) *PatternMatcher {
	sm := make(map[string]manifest.Schema, len(schemas))
	for _, s := range schemas {
		sm[s.Table] = s
	}
	return &PatternMatcher{schemaMap: sm}
}

// TranslateScript converts a Tengo script to Go handler body code.
// It returns the generated Go code and any TODO items for unrecognized patterns.
func (pm *PatternMatcher) TranslateScript(script manifest.Script, route manifest.Route) (string, []string) {
	lines := strings.Split(script.Code, "\n")
	var out []string
	var todos []string

	// Track state for context-aware translation.
	bodyVar := ""     // variable assigned by request.body()
	paramVars := map[string]string{} // varName -> paramName

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		indent := leadingTabs(line, raw)

		// Empty line — pass through.
		if trimmed == "" {
			out = append(out, "")
			continue
		}

		// Closing brace or else — pass through.
		if reCloseBrace.MatchString(trimmed) {
			out = append(out, indent+"}")
			continue
		}
		if reElseUndefined.MatchString(trimmed) {
			out = append(out, indent+"} else {")
			continue
		}

		// if x == undefined { → if err != nil {
		if m := reIfUndefined.FindStringSubmatch(trimmed); m != nil {
			out = append(out, indent+"if err != nil {")
			continue
		}

		// request.param("x") → chi.URLParam
		if m := reRequestParam.FindStringSubmatch(trimmed); m != nil {
			varName, paramName := m[1], m[2]
			paramVars[varName] = paramName
			out = append(out, indent+fmt.Sprintf(`%s := chi.URLParam(r, "%s")`, varName, paramName))
			continue
		}

		// request.query("x") → r.URL.Query().Get("x")
		if m := reRequestQuery.FindStringSubmatch(trimmed); m != nil {
			varName, paramName := m[1], m[2]
			out = append(out, indent+fmt.Sprintf(`%s := r.URL.Query().Get("%s")`, varName, paramName))
			_ = paramName
			continue
		}

		// request.body() → json.NewDecoder decode
		if m := reRequestBody.FindStringSubmatch(trimmed); m != nil {
			varName := m[1]
			bodyVar = varName
			table := inferTableFromRoute(route)
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("var %s model.%s", varName, structName))
			out = append(out, indent+fmt.Sprintf("if err := json.NewDecoder(r.Body).Decode(&%s); err != nil {", varName))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusBadRequest)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			continue
		}

		// db.query("SELECT ... FROM table ...", [params]) → SelectContext
		if m := reDBQueryAssign.FindStringSubmatch(trimmed); m != nil {
			varName, sql, params := m[1], m[2], m[3]
			table := extractTableFromSQL(sql)
			if table == "" {
				table = inferTableFromRoute(route)
			}
			structName := TableToStructName(table)
			goParams := translateParams(params, paramVars)
			out = append(out, indent+fmt.Sprintf("var %s []model.%s", varName, structName))
			out = append(out, indent+fmt.Sprintf(`if err := h.store.DB().SelectContext(ctx, &%s, %q%s); err != nil {`, varName, sql, goParams))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			continue
		}

		// db.query_one("SELECT ... FROM table ...", [params]) → GetContext
		if m := reDBQueryOneAssign.FindStringSubmatch(trimmed); m != nil {
			varName, sql, params := m[1], m[2], m[3]
			table := extractTableFromSQL(sql)
			if table == "" {
				table = inferTableFromRoute(route)
			}
			structName := TableToStructName(table)
			goParams := translateParams(params, paramVars)
			out = append(out, indent+fmt.Sprintf("var %s model.%s", varName, structName))
			out = append(out, indent+fmt.Sprintf(`err := h.store.DB().GetContext(ctx, &%s, %q%s)`, varName, sql, goParams))
			continue
		}

		// db.insert("table", data) → store.CreateX
		if m := reDBInsertAssign.FindStringSubmatch(trimmed); m != nil {
			varName, table := m[1], m[2]
			structName := TableToStructName(table)
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "input"
			}
			out = append(out, indent+fmt.Sprintf("if err := h.store.Create%s(ctx, &%s); err != nil {", structName, dataVar))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			out = append(out, indent+fmt.Sprintf("%s := %s", varName, dataVar))
			continue
		}
		if m := reDBInsertStmt.FindStringSubmatch(trimmed); m != nil {
			table := m[1]
			structName := TableToStructName(table)
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "input"
			}
			out = append(out, indent+fmt.Sprintf("if err := h.store.Create%s(ctx, &%s); err != nil {", structName, dataVar))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			continue
		}

		// db.update("table", id, data) → store.UpdateX
		if m := reDBUpdateAssign.FindStringSubmatch(trimmed); m != nil {
			varName, table, idVar := m[1], m[2], m[3]
			structName := TableToStructName(table)
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "input"
			}
			out = append(out, indent+fmt.Sprintf("if err := h.store.Update%s(ctx, %s, &%s); err != nil {", structName, idVar, dataVar))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			out = append(out, indent+fmt.Sprintf("%s := %s", varName, dataVar))
			continue
		}
		if m := reDBUpdateStmt.FindStringSubmatch(trimmed); m != nil {
			table, idVar := m[1], m[2]
			structName := TableToStructName(table)
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "input"
			}
			out = append(out, indent+fmt.Sprintf("if err := h.store.Update%s(ctx, %s, &%s); err != nil {", structName, idVar, dataVar))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			continue
		}

		// db.delete("table", id) → store.DeleteX
		if m := reDBDeleteAssign.FindStringSubmatch(trimmed); m != nil {
			varName, table, idVar := m[1], m[2], m[3]
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("if err := h.store.Delete%s(ctx, %s); err != nil {", structName, idVar))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			out = append(out, indent+fmt.Sprintf("_ = %s", varName))
			continue
		}
		if m := reDBDeleteStmt.FindStringSubmatch(trimmed); m != nil {
			table, idVar := m[1], m[2]
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("if err := h.store.Delete%s(ctx, %s); err != nil {", structName, idVar))
			out = append(out, indent+"\thttp.Error(w, err.Error(), http.StatusInternalServerError)")
			out = append(out, indent+"\treturn")
			out = append(out, indent+"}")
			continue
		}

		// response.json(data) or response.json(data, status)
		if m := reResponseJSON.FindStringSubmatch(trimmed); m != nil {
			dataVar, statusStr := m[1], m[2]
			out = append(out, indent+`w.Header().Set("Content-Type", "application/json")`)
			if statusStr != "" {
				out = append(out, indent+fmt.Sprintf("w.WriteHeader(%s)", statusStr))
			}
			out = append(out, indent+fmt.Sprintf("json.NewEncoder(w).Encode(%s)", dataVar))
			continue
		}

		// response.fail(status, "msg")
		if m := reResponseFail.FindStringSubmatch(trimmed); m != nil {
			status, msg := m[1], m[2]
			out = append(out, indent+fmt.Sprintf(`http.Error(w, %q, %s)`, msg, status))
			out = append(out, indent+"return")
			continue
		}

		// Unknown patterns — check for complex package calls → TODO stub.
		if reComplexPkg.MatchString(trimmed) {
			todo := fmt.Sprintf("TODO: translate %q", trimmed)
			todos = append(todos, todo)
			out = append(out, indent+fmt.Sprintf("// %s", todo))
			continue
		}

		// Unknown variable assignment — emit a TODO stub.
		if m := reUnknownAssign.FindStringSubmatch(trimmed); m != nil {
			todo := fmt.Sprintf("TODO: translate %q", trimmed)
			todos = append(todos, todo)
			out = append(out, indent+fmt.Sprintf("// %s", todo))
			continue
		}

		// Fall-through: emit as a comment so the file still compiles.
		out = append(out, indent+fmt.Sprintf("// unrecognized: %s", trimmed))
	}

	return strings.Join(out, "\n"), todos
}

// GenerateHandlers produces `package handler` source with a Handler struct
// and one method per route.
func GenerateHandlers(schemas []manifest.Schema, routes []manifest.Route, scripts []manifest.Script) string {
	// Build script name -> Script map.
	scriptMap := make(map[string]manifest.Script, len(scripts))
	for _, s := range scripts {
		scriptMap[s.Name] = s
	}

	pm := NewPatternMatcher(schemas)

	var b strings.Builder

	b.WriteString("package handler\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"strconv\"\n\n")
	b.WriteString("\t\"github.com/go-chi/chi/v5\"\n\n")
	b.WriteString("\t\"{{MODULE}}/internal/model\"\n")
	b.WriteString("\t\"{{MODULE}}/internal/repository\"\n")
	b.WriteString(")\n\n")

	b.WriteString("// Handler holds the application dependencies.\n")
	b.WriteString("type Handler struct {\n")
	b.WriteString("\tstore repository.Store\n")
	b.WriteString("}\n\n")

	b.WriteString("// NewHandler creates a Handler with the provided store.\n")
	b.WriteString("func NewHandler(store repository.Store) *Handler {\n")
	b.WriteString("\treturn &Handler{store: store}\n")
	b.WriteString("}\n\n")

	for _, route := range routes {
		script, ok := scriptMap[route.Script]
		if !ok {
			continue
		}
		methodName := routeToMethodName(route)
		body, todos := pm.TranslateScript(script, route)

		b.WriteString(fmt.Sprintf("// %s handles %s %s\n", methodName, route.Method, route.Path))
		if len(todos) > 0 {
			b.WriteString("// NOTE: this handler has TODOs that require manual attention.\n")
		}
		b.WriteString(fmt.Sprintf("func (h *Handler) %s(w http.ResponseWriter, r *http.Request) {\n", methodName))
		b.WriteString("\tctx := r.Context()\n")

		// Indent the body by one tab.
		for _, line := range strings.Split(body, "\n") {
			if line == "" {
				b.WriteString("\n")
			} else {
				b.WriteString("\t" + line + "\n")
			}
		}

		b.WriteString("}\n\n")
	}

	return b.String()
}

// routeToMethodName converts a route to a Go method name.
// e.g. GET /vehicles → ListVehicles, GET /vehicles/:id → GetVehicle,
// POST /bookings → CreateBooking, PUT /bookings/:id → UpdateBooking,
// DELETE /bookings/:id → DeleteBooking.
func routeToMethodName(route manifest.Route) string {
	// Try route.Script first for a more descriptive name.
	if route.Script != "" {
		return PascalCase(route.Script)
	}

	// Fall back to verb + resource derivation.
	path := route.Path
	// Remove trailing slash.
	path = strings.TrimRight(path, "/")

	segments := strings.Split(path, "/")
	// Find the last non-param segment for the resource name.
	resource := ""
	hasIDParam := false
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "{") {
			hasIDParam = true
		} else {
			resource = seg
		}
	}

	structName := TableToStructName(resource)
	plural := structName + "s"

	switch strings.ToUpper(route.Method) {
	case "GET":
		if hasIDParam {
			return "Get" + structName
		}
		return "List" + plural
	case "POST":
		return "Create" + structName
	case "PUT", "PATCH":
		return "Update" + structName
	case "DELETE":
		return "Delete" + structName
	}
	return PascalCase(route.Method) + structName
}

// chiPath converts `:id` style path params to `{id}` style (chi router).
func chiPath(path string) string {
	re := regexp.MustCompile(`:(\w+)`)
	return re.ReplaceAllString(path, "{$1}")
}

// inferTableFromRoute guesses the table name from a route path.
// e.g. /vehicles/:id → vehicles, /bookings → bookings.
func inferTableFromRoute(route manifest.Route) string {
	path := strings.TrimRight(route.Path, "/")
	segments := strings.Split(path, "/")
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if seg == "" || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "{") {
			continue
		}
		return seg
	}
	return "items"
}

// extractTableFromSQL extracts the first table name from a SQL FROM clause.
func extractTableFromSQL(sql string) string {
	if m := reSQLFrom.FindStringSubmatch(sql); m != nil {
		return m[1]
	}
	return ""
}

// translateParams converts a Tengo param list like "id, true" to Go positional
// args ", id, true" (prefixed with comma+space for appending to a function call).
func translateParams(params string, paramVars map[string]string) string {
	params = strings.TrimSpace(params)
	if params == "" {
		return ""
	}
	parts := strings.Split(params, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return ""
	}
	return ", " + strings.Join(out, ", ")
}

// leadingTabs returns the leading whitespace of a line, normalising spaces to
// tabs at a 2-space ratio so indented code stays readable.
func leadingTabs(line, _ string) string {
	var sb strings.Builder
	for _, ch := range line {
		if ch == '\t' {
			sb.WriteRune('\t')
		} else if ch == ' ' {
			sb.WriteRune(' ')
		} else {
			break
		}
	}
	return sb.String()
}
