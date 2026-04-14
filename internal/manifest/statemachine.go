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

	// Generate a route + script for each transition action.
	for _, tr := range sm.Transitions {
		scriptName := fmt.Sprintf("%s_%s", singular, tr.Action)
		path := fmt.Sprintf("/%s/:id/%s", table, tr.Action)

		code := generateTransitionScript(table, sm.Field, tr)

		routes = append(routes, Route{
			Path:         path,
			Method:       "POST",
			Description:  fmt.Sprintf("Transition %s from %s to %s", singular, tr.From, tr.To),
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

// generateTransitionScript builds a Tengo script for a single state transition.
func generateTransitionScript(table, field string, tr Transition) string {
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
			`auth := request.auth()`,
			fmt.Sprintf(`if auth.role != "%s" {`, tr.Guard.Role),
			fmt.Sprintf(`  response.fail(403, "%s requires %s role")`, tr.Action, tr.Guard.Role),
			`}`,
		)
	}

	// 4. Condition guard.
	if tr.Guard != nil && tr.Guard.Condition != "" {
		lines = append(lines,
			fmt.Sprintf(`if !(%s) {`, tr.Guard.Condition),
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
