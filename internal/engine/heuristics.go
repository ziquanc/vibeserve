package engine

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// actionVerbs are the path-terminal verbs that indicate a state transition.
var actionVerbs = map[string]bool{
	"start": true, "stop": true, "submit": true, "activate": true,
	"deactivate": true, "approve": true, "reject": true, "publish": true,
	"archive": true, "cancel": true, "complete": true, "close": true,
	"open": true, "lock": true, "unlock": true, "pause": true,
	"resume": true, "verify": true, "confirm": true,
}

// aggregationRe matches SQL aggregate keywords (case-insensitive).
var aggregationRe = regexp.MustCompile(`(?i)\b(COUNT|AVG|SUM|MIN|MAX|GROUP\s+BY)\b`)

// ScoreHeuristics analyzes a manifest for architectural patterns beyond basic CRUD.
func ScoreHeuristics(m *manifest.Manifest) HeuristicResult {
	// Build a script lookup by name for fast access.
	scriptByName := make(map[string]string, len(m.Scripts))
	for _, s := range m.Scripts {
		scriptByName[s.Name] = s.Code
	}

	score := 0
	var hints []string
	var suggestions []string

	for _, route := range m.Routes {
		code := scriptByName[route.Script]

		// --- Heuristic 1: State Transitions ---
		if verb, ok := stateTransitionVerb(route.Path); ok {
			score += 2
			hints = append(hints, fmt.Sprintf("State transition detected: %s %s (action: %q)", route.Method, route.Path, verb))
		}

		// --- Heuristic 2: Computed Aggregations ---
		if aggregationRe.MatchString(code) {
			score += 2
			hints = append(hints, fmt.Sprintf("Computed aggregation in script %q (%s %s)", route.Script, route.Method, route.Path))
		}

		// --- Heuristic 3: Validation Guards ---
		if hasValidationGuard(code) {
			score += 1
			hints = append(hints, fmt.Sprintf("Validation guard in script %q (%s %s)", route.Script, route.Method, route.Path))
		}

		// --- Heuristic 4: Lifecycle Hooks ---
		if hasLifecycleHook(route.Path, code) {
			score += 1
			hints = append(hints, fmt.Sprintf("Lifecycle hook in script %q (%s %s)", route.Script, route.Method, route.Path))
		}
	}

	// Cap at 10.
	if score > 10 {
		score = 10
	}

	// Low-score suggestion — only when there are routes to evaluate.
	if score < 4 && len(m.Routes) > 0 {
		suggestions = append(suggestions,
			"Architecture is mostly basic CRUD. Consider adding state transitions, computed endpoints, or validation guards.")
	}

	return HeuristicResult{
		Score:       score,
		Hints:       hints,
		Suggestions: suggestions,
	}
}

// stateTransitionVerb returns the action verb and true if the last non-param
// path segment of p is a known action verb.
func stateTransitionVerb(path string) (string, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	// Walk from end, skip :param segments.
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, ":") {
			continue
		}
		lower := strings.ToLower(seg)
		if actionVerbs[lower] {
			return lower, true
		}
		// The last non-param segment is not a verb — stop.
		break
	}
	return "", false
}

// hasValidationGuard returns true when response.fail( appears before any
// db.insert( or db.update( call in the script.
func hasValidationGuard(code string) bool {
	failIdx := strings.Index(code, "response.fail(")
	if failIdx == -1 {
		return false
	}
	insertIdx := strings.Index(code, "db.insert(")
	updateIdx := strings.Index(code, "db.update(")

	// At least one write must exist AND fail must come before it.
	if insertIdx != -1 && failIdx < insertIdx {
		return true
	}
	if updateIdx != -1 && failIdx < updateIdx {
		return true
	}
	return false
}

// primaryResource extracts the first non-param, non-empty path segment.
func primaryResource(path string) string {
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if seg != "" && !strings.HasPrefix(seg, ":") {
			return strings.ToLower(seg)
		}
	}
	return ""
}

// hasLifecycleHook returns true when a script touches a table other than the
// route's primary resource table (via db.update or db.insert).
var dbOpRe = regexp.MustCompile(`db\.(update|insert)\("([^"]+)"`)

func hasLifecycleHook(routePath, code string) bool {
	primary := primaryResource(routePath)
	if primary == "" {
		return false
	}
	matches := dbOpRe.FindAllStringSubmatch(code, -1)
	for _, m := range matches {
		table := strings.ToLower(m[2])
		if table != primary {
			return true
		}
	}
	return false
}
