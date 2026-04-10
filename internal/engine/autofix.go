package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

const maxAutoFixAttempts = 2

// isCompilationError returns true if the error came from script compilation.
func isCompilationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "compilation:") ||
		strings.Contains(msg, "Compile Error:") ||
		strings.Contains(msg, "script \"")
}

// autoFixManifest attempts to fix script compilation errors by sending the
// failing scripts back to the LLM with a targeted fix prompt. Returns the
// fixed manifest or the original if all attempts fail.
func (e *Engine) autoFixManifest(ctx context.Context, broken *manifest.Manifest, errors []string, stepIndex int, stepDesc string) *manifest.Manifest {
	if e.provider == nil {
		log.Printf("[autofix] no LLM provider, cannot auto-fix")
		return broken
	}

	for attempt := 1; attempt <= maxAutoFixAttempts; attempt++ {
		info := AutoFixInfo{
			Attempt:   attempt,
			Max:       maxAutoFixAttempts,
			Errors:    errors,
			StepIndex: stepIndex,
			StepDesc:  stepDesc,
		}
		e.bus.Publish(Event{Type: EventAutoFixStarted, Data: info})

		log.Printf("[autofix] attempt %d/%d: asking LLM to fix %d script error(s)", attempt, maxAutoFixAttempts, len(errors))

		fixed := e.askLLMToFix(ctx, broken, errors)
		if fixed == nil {
			log.Printf("[autofix] attempt %d: LLM returned nil", attempt)
			info.Fixed = false
			e.bus.Publish(Event{Type: EventAutoFixCompleted, Data: info})
			continue
		}

		// Re-validate the fixed manifest
		remainingErrors := manifest.ValidateCompilationErrors(fixed)
		if len(remainingErrors) == 0 {
			log.Printf("[autofix] attempt %d: all scripts now compile OK", attempt)
			info.Fixed = true
			e.bus.Publish(Event{Type: EventAutoFixCompleted, Data: info})
			return fixed
		}

		log.Printf("[autofix] attempt %d: still %d error(s): %s", attempt, len(remainingErrors), strings.Join(remainingErrors, "; "))
		errors = remainingErrors
		broken = fixed // use partially-fixed manifest as base for next attempt
		info.Fixed = false
		e.bus.Publish(Event{Type: EventAutoFixCompleted, Data: info})
	}

	log.Printf("[autofix] all %d attempts exhausted, returning last version", maxAutoFixAttempts)
	return broken
}

// askLLMToFix builds a targeted prompt that shows the LLM exactly which scripts
// have errors and what those errors are, then asks it to produce a corrected
// manifest.
func (e *Engine) askLLMToFix(ctx context.Context, m *manifest.Manifest, scriptErrors []string) *manifest.Manifest {
	var b strings.Builder

	b.WriteString("The manifest you generated has Tengo script compilation errors. Fix them.\n\n")
	b.WriteString("## Errors\n\n")
	for _, e := range scriptErrors {
		b.WriteString(fmt.Sprintf("- %s\n", e))
	}

	b.WriteString("\n## Tengo Language Rules (CRITICAL)\n\n")
	b.WriteString("- NO top-level `return` statements. Use `response.json(...)` or `response.fail(...)` to send output.\n")
	b.WriteString("- Available globals: `db`, `request`, `response`, `date`, `crypto`, `log`\n")
	b.WriteString("- db methods: db.query(sql, args), db.query_one(sql, args), db.insert(table, data), db.update(table, id, data), db.delete(table, id), db.count(table)\n")
	b.WriteString("- request methods: request.param(key), request.body(), request.header(key), request.query(key)\n")
	b.WriteString("- response methods: response.json(data), response.json(data, statusCode), response.fail(statusCode, message), response.set_header(key, value)\n")
	b.WriteString("- log methods: log.info(msg), log.warn(msg), log.err(msg)\n")
	b.WriteString("- date: date.now() returns datetime string, date.format(fmt, ts) formats timestamp\n")
	b.WriteString("- Use `is_undefined(x)` instead of `x == undefined` to check for undefined values\n")
	b.WriteString("- String functions: len(s), strings.contains(s, substr), strings.split(s, sep), strings.replace(s, old, new), strings.trim(s), strings.upper(s), strings.lower(s), strings.has_prefix(s, prefix), strings.has_suffix(s, suffix), strings.index(s, substr)\n")
	b.WriteString("- DO NOT use `strlen()` — use `len(s)` instead\n")
	b.WriteString("- DO NOT use `return` at top level — just call response methods\n")
	b.WriteString("- Math: math.abs(x), math.ceil(x), math.floor(x), math.max(a,b), math.min(a,b), math.pow(x,y), math.sqrt(x)\n")
	b.WriteString("- Convert types: int(x), float(x), string(x), bool(x)\n")
	b.WriteString("- `fmt` module is NOT available. Use string concatenation with `+` for formatting.\n")
	b.WriteString("- `json` module is NOT available. Use response.json() to output JSON.\n")

	b.WriteString("\n## Current Manifest\n\n")
	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Printf("[autofix] failed to marshal manifest: %v", err)
		return nil
	}
	b.WriteString(string(manifestJSON))

	b.WriteString("\n\n## Instructions\n\n")
	b.WriteString("Output the COMPLETE corrected manifest as JSON. Fix ALL script compilation errors. Do NOT change the schema, routes, or seeds — only fix script code.\n")
	b.WriteString("Output ONLY valid JSON. No markdown fences, no explanation.\n")

	fixed, err := e.provider.Generate(ctx, m, b.String(), nil)
	if err != nil {
		log.Printf("[autofix] LLM fix generation failed: %v", err)
		return nil
	}

	return fixed
}
