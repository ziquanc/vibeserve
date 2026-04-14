package manifest

import (
	"fmt"
	"strings"
)

// StateMachine defines a finite state machine on a schema field.
type StateMachine struct {
	Field       string       `json:"field"`
	Initial     string       `json:"initial"`
	Transitions []Transition `json:"transitions"`
}

// Transition represents a single allowed state change.
type Transition struct {
	From   string           `json:"from"`
	To     string           `json:"to"`
	Action string           `json:"action"`
	Guard  *TransitionGuard `json:"guard,omitempty"`
}

// TransitionGuard optionally restricts when a transition may fire.
type TransitionGuard struct {
	Role      string `json:"role,omitempty"`
	Condition string `json:"condition,omitempty"`
}

// ValidStates returns all unique states referenced by the state machine
// (the initial state plus every from/to in the transitions).
func (sm *StateMachine) ValidStates() []string {
	seen := make(map[string]bool)
	var states []string

	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			states = append(states, s)
		}
	}

	add(sm.Initial)
	for _, tr := range sm.Transitions {
		add(tr.From)
		add(tr.To)
	}
	return states
}

// TransitionsFrom returns all transitions whose From field matches the given state.
func (sm *StateMachine) TransitionsFrom(state string) []Transition {
	var result []Transition
	for _, tr := range sm.Transitions {
		if tr.From == state {
			result = append(result, tr)
		}
	}
	return result
}

// GenerateTransitionRoutes produces routes and scripts for every transition
// in the state machine, plus a GET route that lists available transitions
// for the current state of a row.
func GenerateTransitionRoutes(table string, sm *StateMachine) ([]Route, []Script) {
	singular := singularize(table)
	var routes []Route
	var scripts []Script

	// Group transitions by action — same action from different states becomes one route.
	actionGroups := make(map[string][]Transition)
	var actionOrder []string
	for _, tr := range sm.Transitions {
		if _, exists := actionGroups[tr.Action]; !exists {
			actionOrder = append(actionOrder, tr.Action)
		}
		actionGroups[tr.Action] = append(actionGroups[tr.Action], tr)
	}

	for _, action := range actionOrder {
		transitions := actionGroups[action]
		scriptName := fmt.Sprintf("%s_%s", singular, action)
		path := fmt.Sprintf("/%s/:id/%s", table, action)

		// Build description from all transitions for this action
		var descs []string
		for _, tr := range transitions {
			descs = append(descs, fmt.Sprintf("%s→%s", tr.From, tr.To))
		}
		desc := fmt.Sprintf("Transition %s: %s", singular, strings.Join(descs, " or "))

		code := generateTransitionScript(table, sm.Field, transitions)

		routes = append(routes, Route{
			Path:         path,
			Method:       "POST",
			Description:  desc,
			Script:       scriptName,
			ResponseType: "object",
		})
		scripts = append(scripts, Script{
			Name: scriptName,
			Code: code,
		})
	}

	// Generate the transitions-list route.
	listScriptName := fmt.Sprintf("%s_transitions", singular)
	listPath := fmt.Sprintf("/%s/:id/transitions", table)
	listCode := generateTransitionsListScript(table, sm)

	routes = append(routes, Route{
		Path:         listPath,
		Method:       "GET",
		Description:  fmt.Sprintf("List available transitions for a %s", singular),
		Script:       listScriptName,
		ResponseType: "object",
	})
	scripts = append(scripts, Script{
		Name: listScriptName,
		Code: listCode,
	})

	return routes, scripts
}

// generateTransitionScript builds a Tengo script for one or more transitions
// sharing the same action (e.g., "cancel" from pending AND from shipped).
func generateTransitionScript(table, field string, transitions []Transition) string {
	if len(transitions) == 1 {
		return generateSingleTransitionScript(table, field, transitions[0])
	}

	// Multiple from-states for the same action — generate if/elseif chain
	var lines []string
	lines = append(lines,
		`id := request.param("id")`,
		fmt.Sprintf(`row := db.query_one("SELECT * FROM %s WHERE id = ? AND deleted_at IS NULL", [id])`, table),
		`if row == undefined {`,
		fmt.Sprintf(`  response.fail(404, "%s not found")`, singularize(table)),
		`}`,
	)

	// Build valid from-states list for error message
	var validStates []string
	for _, tr := range transitions {
		validStates = append(validStates, tr.From)
	}

	for i, tr := range transitions {
		keyword := "if"
		if i > 0 {
			keyword = "} else if"
		}
		lines = append(lines, fmt.Sprintf(`%s row.%s == "%s" {`, keyword, field, tr.From))

		// Role guard
		if tr.Guard != nil && tr.Guard.Role != "" {
			lines = append(lines,
				`  user_auth := request.auth()`,
				`  if user_auth == undefined {`,
				`    response.fail(401, "authentication required")`,
				`  }`,
				fmt.Sprintf(`  if user_auth.role != "%s" {`, tr.Guard.Role),
				fmt.Sprintf(`    response.fail(403, "%s requires %s role")`, tr.Action, tr.Guard.Role),
				`  }`,
			)
		}

		// Condition guard
		if tr.Guard != nil && tr.Guard.Condition != "" {
			condition := prefixRowFields(tr.Guard.Condition)
			lines = append(lines,
				fmt.Sprintf(`  if !(%s) {`, condition),
				fmt.Sprintf(`    response.fail(400, "guard condition not met: %s")`, tr.Guard.Condition),
				`  }`,
			)
		}

		lines = append(lines,
			fmt.Sprintf(`  db.query("UPDATE %s SET %s = '%s', updated_at = date.now() WHERE id = ?", [id])`, table, field, tr.To),
		)
	}

	lines = append(lines,
		`} else {`,
		fmt.Sprintf(`  response.fail(400, "cannot %s from current state; valid states: %s")`, transitions[0].Action, strings.Join(validStates, ", ")),
		`}`,
		fmt.Sprintf(`result := db.query_one("SELECT * FROM %s WHERE id = ?", [id])`, table),
		`response.json(result)`,
	)

	return joinLines(lines)
}

// generateSingleTransitionScript builds a Tengo script for a single state transition.
func generateSingleTransitionScript(table, field string, tr Transition) string {
	var lines []string

	// 1. Fetch row by id.
	lines = append(lines,
		`id := request.param("id")`,
		fmt.Sprintf(`row := db.query_one("SELECT * FROM %s WHERE id = ? AND deleted_at IS NULL", [id])`, table),
		`if row == undefined {`,
		fmt.Sprintf(`  response.fail(404, "%s not found")`, singularize(table)),
		`}`,
	)

	// 2. Check current state matches "from".
	lines = append(lines,
		fmt.Sprintf(`if row.%s != "%s" {`, field, tr.From),
		fmt.Sprintf(`  response.fail(400, "cannot %s: current status is not %s")`, tr.Action, tr.From),
		`}`,
	)

	// 3. Role guard.
	if tr.Guard != nil && tr.Guard.Role != "" {
		lines = append(lines,
			`user_auth := request.auth()`,
			`if user_auth == undefined {`,
			`  response.fail(401, "authentication required")`,
			`}`,
			fmt.Sprintf(`if user_auth.role != "%s" {`, tr.Guard.Role),
			fmt.Sprintf(`  response.fail(403, "%s requires %s role")`, tr.Action, tr.Guard.Role),
			`}`,
		)
	}

	// 4. Condition guard — prefix field names with "row." for access.
	if tr.Guard != nil && tr.Guard.Condition != "" {
		// Wrap the condition to reference the row object.
		// e.g., "total > 0" becomes "row.total > 0"
		condition := prefixRowFields(tr.Guard.Condition)
		lines = append(lines,
			fmt.Sprintf(`if !(%s) {`, condition),
			fmt.Sprintf(`  response.fail(400, "guard condition not met: %s")`, tr.Guard.Condition),
			`}`,
		)
	}

	// 5. UPDATE status and updated_at.
	lines = append(lines,
		fmt.Sprintf(`db.query_one("UPDATE %s SET %s = ?, updated_at = ? WHERE id = ? RETURNING *", ["%s", date.now(), id])`, table, field, tr.To),
		fmt.Sprintf(`updated := db.query_one("SELECT * FROM %s WHERE id = ?", [id])`, table),
		`response.json(updated)`,
	)

	return joinLines(lines)
}

// generateTransitionsListScript builds a Tengo script that returns the current
// status and available transitions for a row.
func generateTransitionsListScript(table string, sm *StateMachine) string {
	var lines []string

	// Fetch row.
	lines = append(lines,
		`id := request.param("id")`,
		fmt.Sprintf(`row := db.query_one("SELECT * FROM %s WHERE id = ? AND deleted_at IS NULL", [id])`, table),
		`if row == undefined {`,
		fmt.Sprintf(`  response.fail(404, "%s not found")`, singularize(table)),
		`}`,
		fmt.Sprintf(`status := row.%s`, sm.Field),
	)

	// Build transitions array based on current status.
	lines = append(lines, `transitions := []`)

	// Group transitions by from-state for cleaner script output.
	fromStates := make(map[string][]Transition)
	var orderedFromStates []string
	for _, tr := range sm.Transitions {
		if _, exists := fromStates[tr.From]; !exists {
			orderedFromStates = append(orderedFromStates, tr.From)
		}
		fromStates[tr.From] = append(fromStates[tr.From], tr)
	}

	for _, state := range orderedFromStates {
		trs := fromStates[state]
		lines = append(lines, fmt.Sprintf(`if status == "%s" {`, state))
		for _, tr := range trs {
			lines = append(lines, fmt.Sprintf(`  transitions = append(transitions, {action: "%s", to: "%s"})`, tr.Action, tr.To))
		}
		lines = append(lines, `}`)
	}

	lines = append(lines, `response.json({status: status, transitions: transitions})`)

	return joinLines(lines)
}

// singularize performs a naive conversion of a plural English word to singular.
// This is a local helper to avoid importing the engine package (circular dep).
func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "ses") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

// joinLines concatenates lines with newline separators.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// prefixRowFields adds "row." prefix to identifiers in a condition string.
// "total > 0" becomes "row.total > 0"
// "items_count > 0" becomes "row.items_count > 0"
// Already-prefixed "row.x" is left unchanged.
func prefixRowFields(condition string) string {
	// Simple approach: split by spaces, prefix words that look like field names
	words := strings.Fields(condition)
	for i, w := range words {
		// Skip operators, numbers, strings
		if w == ">" || w == "<" || w == ">=" || w == "<=" || w == "==" || w == "!=" || w == "&&" || w == "||" || w == "!" {
			continue
		}
		// Skip numbers
		if len(w) > 0 && (w[0] >= '0' && w[0] <= '9') {
			continue
		}
		// Skip already prefixed
		if strings.HasPrefix(w, "row.") {
			continue
		}
		// Skip keywords
		if w == "true" || w == "false" || w == "undefined" {
			continue
		}
		// This looks like a field name — prefix it
		if len(w) > 0 && ((w[0] >= 'a' && w[0] <= 'z') || w[0] == '_') {
			words[i] = "row." + w
		}
	}
	return strings.Join(words, " ")
}
