# Proposal Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a blueprint review phase to `vibeserve` so the engine proposes changes before applying them, with architectural heuristic scoring and a read-only web preview.

**Architecture:** The engine's `Apply()` method splits into two phases: propose (generate manifest, diff, score heuristics, pause) and apply (on user approval). A new `proposalPending` state in the TUI routes input to approve/refine/cancel. A read-only web page at `/_blueprint` renders the pending blueprint.

**Tech Stack:** Go, Bubble Tea v2, embedded HTML (same patterns as existing `/_console/`).

---

## File Structure

### New files to create:

| File | Responsibility |
|---|---|
| `internal/engine/blueprint.go` | `BlueprintInfo`, `BlueprintResult`, `HeuristicResult` types + `ApproveBlueprint()`, `RefineBlueprint()`, `CancelBlueprint()` methods |
| `internal/engine/blueprint_test.go` | Tests for blueprint lifecycle |
| `internal/engine/heuristics.go` | `ScoreHeuristics()` — pattern matching on routes/scripts |
| `internal/engine/heuristics_test.go` | Tests for each heuristic category |
| `internal/web/blueprint.go` | `GET /_api/blueprint` handler |
| `internal/web/static/blueprint.html` | Embedded read-only blueprint preview page |

### Files to modify:

| File | Change |
|---|---|
| `internal/engine/engine.go` | Add `pendingBlueprint` field, modify `Apply()` to return `BlueprintResult`, extract `commitManifest()` |
| `internal/engine/events.go` | Add 3 new event types + `BlueprintInfo` reference |
| `internal/llm/provider.go` | Append Architectural Heuristics to `BuildSystemPrompt()` |
| `internal/tui/messages.go` | Add 4 new TUI message types |
| `internal/tui/conversation.go` | Add `proposalPending` state, `blueprint>` prompt, route y/n/enhance |
| `internal/tui/app.go` | Handle `BlueprintProposedMsg`, `ApproveBlueprint`, `RefineBlueprint` flows |
| `internal/tui/bridge.go` | Subscribe to 3 new blueprint events |
| `internal/web/mux.go` | Register `/_blueprint` and `/_api/blueprint` routes |
| `internal/web/console.go` | Add blueprint handler to `RegisterRoutes` |

---

### Task 1: Types, Events, and Messages

**Files:**
- Create: `internal/engine/blueprint.go`
- Modify: `internal/engine/events.go`
- Modify: `internal/tui/messages.go`

- [ ] **Step 1: Write blueprint types**

```go
// internal/engine/blueprint.go
package engine

import "github.com/vibeserve/vibeserve/internal/manifest"

// HeuristicResult holds the architectural quality analysis of a blueprint.
type HeuristicResult struct {
	Score       int      // 0-10, higher = more architectural depth
	Hints       []string // non-CRUD patterns detected (shown as highlights)
	Suggestions []string // improvements the user could request
}

// BlueprintInfo holds a proposed manifest with its analysis.
type BlueprintInfo struct {
	Manifest   *manifest.Manifest
	Changes    []manifest.Change
	Warnings   []string
	Summary    string          // human-readable change summary
	Heuristics HeuristicResult // architectural quality score
}

// BlueprintResult is returned by Apply() in proposal mode.
type BlueprintResult struct {
	Blueprint    *BlueprintInfo // non-nil when a proposal is pending
	ChatResponse string         // non-empty if LLM responded conversationally
}
```

- [ ] **Step 2: Add new event types to events.go**

In `internal/engine/events.go`, after the line `EventStepCompleted EventType = "STEP_COMPLETED"`, add:

```go
	EventBlueprintProposed EventType = "BLUEPRINT_PROPOSED"
	EventBlueprintRefined  EventType = "BLUEPRINT_REFINED"
	EventBlueprintApproved EventType = "BLUEPRINT_APPROVED"
```

- [ ] **Step 3: Add new TUI message types to messages.go**

In `internal/tui/messages.go`, after the `LogMsg` struct (end of file), add:

```go
// BlueprintProposedMsg is sent when the engine proposes a blueprint for review.
type BlueprintProposedMsg struct {
	Blueprint *engine.BlueprintInfo
}

// BlueprintApproveMsg is sent when the user approves the pending blueprint.
type BlueprintApproveMsg struct{}

// BlueprintRefineMsg is sent when the user wants to refine the blueprint.
type BlueprintRefineMsg struct {
	Feedback string
}

// BlueprintCancelMsg is sent when the user cancels the pending blueprint.
type BlueprintCancelMsg struct{}

// BlueprintAppliedMsg is sent when the approved blueprint has been applied.
type BlueprintAppliedMsg struct {
	Result *engine.ApplyResult
	Err    error
}
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./...`
Expected: Build succeeds (new types are defined but not yet used)

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/engine/blueprint.go internal/engine/events.go internal/tui/messages.go
git commit -m "feat(blueprint): add types, events, and messages for proposal mode"
```

---

### Task 2: Heuristic Scoring

**Files:**
- Create: `internal/engine/heuristics.go`
- Create: `internal/engine/heuristics_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/engine/heuristics_test.go
package engine

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestScoreHeuristics_PureCRUD(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items"},
			{Path: "/items/:id", Method: "GET", Script: "get_item"},
			{Path: "/items", Method: "POST", Script: "create_item"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: `result := db.query("SELECT * FROM items", [])` + "\n" + `response.json(result)`},
			{Name: "get_item", Code: `id := request.param("id")` + "\n" + `row := db.query_one("SELECT * FROM items WHERE id = ?", [id])` + "\n" + `response.json(row)`},
			{Name: "create_item", Code: `body := request.body()` + "\n" + `result := db.insert("items", body)` + "\n" + `response.json(result, 201)`},
		},
	}

	result := ScoreHeuristics(m)

	if result.Score > 3 {
		t.Errorf("pure CRUD should score low, got %d", result.Score)
	}
	if len(result.Suggestions) == 0 {
		t.Error("pure CRUD should have suggestions")
	}
}

func TestScoreHeuristics_StateTransition(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/exams/:id/start", Method: "POST", Script: "start_exam"},
			{Path: "/exams/:id/submit", Method: "PATCH", Script: "submit_exam"},
		},
		Scripts: []manifest.Script{
			{Name: "start_exam", Code: `db.update("exams", id, {status: "in_progress"})`},
			{Name: "submit_exam", Code: `db.update("exams", id, {status: "completed"})`},
		},
	}

	result := ScoreHeuristics(m)

	if result.Score < 2 {
		t.Errorf("state transitions should score >= 2, got %d", result.Score)
	}
	foundHint := false
	for _, h := range result.Hints {
		if len(h) > 0 {
			foundHint = true
		}
	}
	if !foundHint {
		t.Error("should have hints about state transitions")
	}
}

func TestScoreHeuristics_ComputedAggregation(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/students/:id/progress", Method: "GET", Script: "student_progress"},
		},
		Scripts: []manifest.Script{
			{Name: "student_progress", Code: `result := db.query("SELECT AVG(score) as avg_score, COUNT(*) as total FROM submissions WHERE student_id = ?", [id])` + "\n" + `response.json(result)`},
		},
	}

	result := ScoreHeuristics(m)

	if result.Score < 2 {
		t.Errorf("aggregation should score >= 2, got %d", result.Score)
	}
}

func TestScoreHeuristics_ValidationGuard(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/exams/:id/submit", Method: "POST", Script: "submit_exam"},
		},
		Scripts: []manifest.Script{
			{Name: "submit_exam", Code: `exam := db.query_one("SELECT * FROM exams WHERE id = ?", [id])` + "\n" + `if exam.status != "in_progress" {` + "\n" + `  response.fail(400, "Exam not in progress")` + "\n" + `}` + "\n" + `db.update("exams", id, {status: "submitted"})`},
		},
	}

	result := ScoreHeuristics(m)

	if result.Score < 1 {
		t.Errorf("validation guard should score >= 1, got %d", result.Score)
	}
}

func TestScoreHeuristics_LifecycleHook(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/submissions", Method: "POST", Script: "submit_answer"},
		},
		Scripts: []manifest.Script{
			{Name: "submit_answer", Code: `body := request.body()` + "\n" + `result := db.insert("submissions", body)` + "\n" + `db.update("students", body.student_id, {last_submission: date.now()})` + "\n" + `response.json(result, 201)`},
		},
	}

	result := ScoreHeuristics(m)

	if result.Score < 1 {
		t.Errorf("lifecycle hook should score >= 1, got %d", result.Score)
	}
}

func TestScoreHeuristics_CapsAt10(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/a/:id/start", Method: "POST", Script: "s1"},
			{Path: "/a/:id/stop", Method: "POST", Script: "s2"},
			{Path: "/b/:id/activate", Method: "POST", Script: "s3"},
			{Path: "/b/:id/deactivate", Method: "POST", Script: "s4"},
			{Path: "/c/:id/approve", Method: "PATCH", Script: "s5"},
			{Path: "/stats", Method: "GET", Script: "s6"},
		},
		Scripts: []manifest.Script{
			{Name: "s1", Code: "db.update(\"a\", id, {})"},
			{Name: "s2", Code: "db.update(\"a\", id, {})"},
			{Name: "s3", Code: "db.update(\"b\", id, {})"},
			{Name: "s4", Code: "db.update(\"b\", id, {})"},
			{Name: "s5", Code: "response.fail(400, \"nope\")\ndb.update(\"c\", id, {})"},
			{Name: "s6", Code: `db.query("SELECT COUNT(*) FROM x", [])`},
		},
	}

	result := ScoreHeuristics(m)

	if result.Score > 10 {
		t.Errorf("score should cap at 10, got %d", result.Score)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/engine/ -run "TestScoreHeuristics" -v`
Expected: FAIL — ScoreHeuristics not defined

- [ ] **Step 3: Implement heuristic scoring**

```go
// internal/engine/heuristics.go
package engine

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Action verbs that indicate state transitions (appended to resource path).
var stateTransitionVerbs = []string{
	"start", "stop", "submit", "activate", "deactivate",
	"approve", "reject", "publish", "archive", "cancel",
	"complete", "close", "open", "lock", "unlock",
	"pause", "resume", "verify", "confirm",
}

var reAggregateSQL = regexp.MustCompile(`(?i)\b(COUNT|AVG|SUM|MIN|MAX|GROUP\s+BY)\b`)

// ScoreHeuristics analyzes a manifest for architectural depth beyond CRUD.
func ScoreHeuristics(m *manifest.Manifest) HeuristicResult {
	var score int
	var hints []string
	var suggestions []string

	scriptMap := make(map[string]string)
	for _, s := range m.Scripts {
		scriptMap[s.Name] = s.Code
	}

	// 1. State Transitions: routes with action verbs on a resource
	for _, r := range m.Routes {
		segments := strings.Split(strings.Trim(r.Path, "/"), "/")
		if len(segments) < 2 {
			continue
		}
		lastSeg := segments[len(segments)-1]
		if strings.HasPrefix(lastSeg, ":") {
			continue // parameter, not an action
		}
		for _, verb := range stateTransitionVerbs {
			if strings.EqualFold(lastSeg, verb) {
				score += 2
				hints = append(hints, fmt.Sprintf("State transition: %s %s", r.Method, r.Path))
				break
			}
		}
	}

	// 2. Computed Aggregations: scripts with COUNT/AVG/SUM/GROUP BY
	for _, r := range m.Routes {
		code, ok := scriptMap[r.Script]
		if !ok {
			continue
		}
		if reAggregateSQL.MatchString(code) {
			score += 2
			hints = append(hints, fmt.Sprintf("Computed aggregation: %s %s", r.Method, r.Path))
		}
	}

	// 3. Validation Guards: scripts with response.fail() before main db operation
	for _, r := range m.Routes {
		code, ok := scriptMap[r.Script]
		if !ok {
			continue
		}
		failIdx := strings.Index(code, "response.fail(")
		if failIdx < 0 {
			continue
		}
		// Check if there's a db.insert/db.update AFTER the fail guard
		afterFail := code[failIdx:]
		if strings.Contains(afterFail, "db.insert(") || strings.Contains(afterFail, "db.update(") {
			score += 1
			hints = append(hints, fmt.Sprintf("Validation guard: %s %s", r.Method, r.Path))
		}
	}

	// 4. Lifecycle Hooks: scripts that modify a different table than route's primary
	for _, r := range m.Routes {
		code, ok := scriptMap[r.Script]
		if !ok {
			continue
		}
		primaryTable := inferPrimaryTable(r.Path)
		if primaryTable == "" {
			continue
		}
		// Find all db.update/db.insert calls referencing OTHER tables
		reOtherTable := regexp.MustCompile(`db\.(update|insert)\("(\w+)"`)
		matches := reOtherTable.FindAllStringSubmatch(code, -1)
		for _, match := range matches {
			if match[2] != primaryTable {
				score += 1
				hints = append(hints, fmt.Sprintf("Lifecycle hook: %s %s updates %s", r.Method, r.Path, match[2]))
				break
			}
		}
	}

	// Cap at 10
	if score > 10 {
		score = 10
	}

	// Suggestions for low scores
	if score < 4 {
		suggestions = append(suggestions, "Architecture is mostly basic CRUD. Consider adding state transitions, computed endpoints, or validation guards.")
	}

	return HeuristicResult{
		Score:       score,
		Hints:       hints,
		Suggestions: suggestions,
	}
}

// inferPrimaryTable guesses the primary table from a route path.
func inferPrimaryTable(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for _, seg := range segments {
		if !strings.HasPrefix(seg, ":") {
			return seg
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/engine/ -run "TestScoreHeuristics" -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/engine/heuristics.go internal/engine/heuristics_test.go
git commit -m "feat(blueprint): add heuristic scoring — state transitions, aggregations, guards, hooks"
```

---

### Task 3: Engine Blueprint Lifecycle

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/blueprint.go`
- Create: `internal/engine/blueprint_test.go`

- [ ] **Step 1: Write failing tests for blueprint lifecycle**

```go
// internal/engine/blueprint_test.go
package engine

import (
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestEngine_HasPendingBlueprint(t *testing.T) {
	eng := &Engine{}
	if eng.HasPendingBlueprint() {
		t.Error("should not have pending blueprint initially")
	}

	eng.pendingBlueprint = &BlueprintInfo{
		Manifest: &manifest.Manifest{Name: "test"},
	}
	if !eng.HasPendingBlueprint() {
		t.Error("should have pending blueprint after setting")
	}
}

func TestEngine_CancelBlueprint(t *testing.T) {
	eng := &Engine{}
	eng.pendingBlueprint = &BlueprintInfo{
		Manifest: &manifest.Manifest{Name: "test"},
	}

	eng.CancelBlueprint()

	if eng.HasPendingBlueprint() {
		t.Error("should not have pending blueprint after cancel")
	}
}

func TestEngine_PendingBlueprint(t *testing.T) {
	eng := &Engine{}
	if eng.PendingBlueprint() != nil {
		t.Error("should return nil when no pending blueprint")
	}

	bp := &BlueprintInfo{
		Manifest: &manifest.Manifest{Name: "test"},
		Heuristics: HeuristicResult{Score: 5},
	}
	eng.pendingBlueprint = bp

	got := eng.PendingBlueprint()
	if got == nil {
		t.Fatal("should return pending blueprint")
	}
	if got.Manifest.Name != "test" {
		t.Errorf("manifest name = %q, want %q", got.Manifest.Name, "test")
	}
	if got.Heuristics.Score != 5 {
		t.Errorf("score = %d, want 5", got.Heuristics.Score)
	}
}

func TestFormatBlueprintSummary(t *testing.T) {
	bp := &BlueprintInfo{
		Manifest: &manifest.Manifest{
			Name: "test",
			Schemas: []manifest.Schema{
				{Table: "items"},
				{Table: "orders"},
			},
			Routes: []manifest.Route{
				{Path: "/items", Method: "GET"},
				{Path: "/items/:id/activate", Method: "POST"},
			},
			Scripts: []manifest.Script{
				{Name: "s1"}, {Name: "s2"},
			},
		},
		Heuristics: HeuristicResult{
			Score: 6,
			Hints: []string{"State transition: POST /items/:id/activate"},
		},
	}

	summary := FormatBlueprintSummary(bp)

	if summary == "" {
		t.Error("summary should not be empty")
	}
	if !containsStr(summary, "score: 6/10") {
		t.Error("should contain score")
	}
	if !containsStr(summary, "2 routes") {
		t.Error("should contain route count")
	}
	if !containsStr(summary, "2 tables") {
		t.Error("should contain table count")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > len(sub) && searchStr(s, sub))
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/engine/ -run "TestEngine_HasPending|TestEngine_Cancel|TestEngine_Pending|TestFormatBlueprint" -v`
Expected: FAIL — methods not defined

- [ ] **Step 3: Add pendingBlueprint field to Engine**

In `internal/engine/engine.go`, add to the Engine struct (after `storeOpener`):

```go
	pendingBlueprint *BlueprintInfo
```

- [ ] **Step 4: Implement blueprint methods in blueprint.go**

Append to `internal/engine/blueprint.go`:

```go
import (
	"context"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)
```

Update the import block to include the above, then add after the existing types:

```go
// HasPendingBlueprint returns true if a blueprint is awaiting approval.
func (e *Engine) HasPendingBlueprint() bool {
	return e.pendingBlueprint != nil
}

// PendingBlueprint returns the current pending blueprint, or nil.
func (e *Engine) PendingBlueprint() *BlueprintInfo {
	return e.pendingBlueprint
}

// CancelBlueprint discards the pending blueprint.
func (e *Engine) CancelBlueprint() {
	e.pendingBlueprint = nil
}

// ProposeBlueprint generates a blueprint from a new manifest without applying it.
// Called internally by Apply() to enter proposal mode.
func (e *Engine) proposeBlueprint(newManifest *manifest.Manifest) (*BlueprintInfo, error) {
	// Repair
	repairManifest(newManifest, e.manifest)

	// Validate
	if err := manifest.Validate(newManifest); err != nil {
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	// Diff
	changes := manifest.Diff(e.manifest, newManifest)

	// Heuristics
	heuristics := ScoreHeuristics(newManifest)

	// Summary
	summary := FormatChangeSummaryFromChanges(changes)

	// Check for breaking changes
	var warnings []string
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeDropColumn:
			warnings = append(warnings, fmt.Sprintf("Breaking: %s", c.Detail))
		case manifest.ChangeRemoveRoute:
			warnings = append(warnings, fmt.Sprintf("Breaking: %s", c.Detail))
		case manifest.ChangeRemoveScript:
			warnings = append(warnings, fmt.Sprintf("Changed: %s", c.Detail))
		}
	}

	bp := &BlueprintInfo{
		Manifest:   newManifest,
		Changes:    changes,
		Warnings:   warnings,
		Summary:    summary,
		Heuristics: heuristics,
	}

	e.pendingBlueprint = bp
	return bp, nil
}

// ApproveBlueprint applies the pending blueprint.
func (e *Engine) ApproveBlueprint(ctx context.Context) (*ApplyResult, error) {
	if e.pendingBlueprint == nil {
		return nil, fmt.Errorf("no pending blueprint to approve")
	}

	bp := e.pendingBlueprint
	e.pendingBlueprint = nil

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventBlueprintApproved, Data: *bp})
	}

	// Apply the manifest using the existing applyManifest logic
	result := &ApplyResult{}
	result, err := e.applyManifest(ctx, "", bp.Manifest, result)
	return result, err
}

// RefineBlueprint sends feedback to LLM and generates a new blueprint.
func (e *Engine) RefineBlueprint(ctx context.Context, feedback string) (*BlueprintInfo, error) {
	if e.pendingBlueprint == nil {
		return nil, fmt.Errorf("no pending blueprint to refine")
	}

	if e.provider == nil {
		return nil, fmt.Errorf("no LLM provider configured")
	}

	// Send feedback to LLM with current blueprint context
	prompt := fmt.Sprintf("Refine the current API design based on this feedback: %s", feedback)

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestStarted, Data: "Refining blueprint..."})
	}

	newManifest, err := e.provider.Generate(ctx, e.pendingBlueprint.Manifest, prompt, e.history)
	if err != nil {
		return nil, fmt.Errorf("LLM refinement failed: %w", err)
	}

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventLLMRequestCompleted})
	}

	bp, err := e.proposeBlueprint(newManifest)
	if err != nil {
		return nil, err
	}

	if e.bus != nil {
		e.bus.Publish(Event{Type: EventBlueprintRefined, Data: *bp})
	}

	return bp, nil
}

// FormatChangeSummaryFromChanges creates a human-readable summary from a change list.
func FormatChangeSummaryFromChanges(changes []manifest.Change) string {
	var tables, routes, scripts, seeds int
	for _, c := range changes {
		switch c.Type {
		case manifest.ChangeAddTable, manifest.ChangeAddColumn, manifest.ChangeDropColumn:
			tables++
		case manifest.ChangeAddRoute, manifest.ChangeUpdateRoute, manifest.ChangeRemoveRoute:
			routes++
		case manifest.ChangeAddScript, manifest.ChangeUpdateScript, manifest.ChangeRemoveScript:
			scripts++
		case manifest.ChangeAddSeed:
			seeds++
		}
	}
	parts := []string{}
	if tables > 0 {
		parts = append(parts, fmt.Sprintf("%d schema changes", tables))
	}
	if routes > 0 {
		parts = append(parts, fmt.Sprintf("%d route changes", routes))
	}
	if scripts > 0 {
		parts = append(parts, fmt.Sprintf("%d script changes", scripts))
	}
	if seeds > 0 {
		parts = append(parts, fmt.Sprintf("%d seed operations", seeds))
	}
	if len(parts) == 0 {
		return "No changes"
	}
	return strings.Join(parts, ", ")
}

// FormatBlueprintSummary creates a TUI-friendly summary of a blueprint.
func FormatBlueprintSummary(bp *BlueprintInfo) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Blueprint ready (score: %d/10)\n", bp.Heuristics.Score))
	b.WriteString("\n")

	if bp.Manifest != nil {
		b.WriteString(fmt.Sprintf("  %d tables, %d routes, %d scripts\n",
			len(bp.Manifest.Schemas), len(bp.Manifest.Routes), len(bp.Manifest.Scripts)))
	}

	if len(bp.Warnings) > 0 {
		b.WriteString("\n")
		for _, w := range bp.Warnings {
			b.WriteString(fmt.Sprintf("  \u26a0 %s\n", w))
		}
	}

	if len(bp.Heuristics.Hints) > 0 {
		b.WriteString("\n")
		for _, h := range bp.Heuristics.Hints {
			b.WriteString(fmt.Sprintf("  \u2726 %s\n", h))
		}
	}

	if len(bp.Heuristics.Suggestions) > 0 {
		b.WriteString("\n")
		for _, s := range bp.Heuristics.Suggestions {
			b.WriteString(fmt.Sprintf("  \u26a0 %s\n", s))
		}
	}

	return b.String()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./internal/engine/ -run "TestEngine_HasPending|TestEngine_Cancel|TestEngine_Pending|TestFormatBlueprint" -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/engine/blueprint.go internal/engine/blueprint_test.go internal/engine/engine.go
git commit -m "feat(blueprint): add engine blueprint lifecycle — propose, approve, refine, cancel"
```

---

### Task 4: Modify Engine.Apply() for Proposal Mode

**Files:**
- Modify: `internal/engine/engine.go`

- [ ] **Step 1: Read current Apply() and applyManifest()**

Read `internal/engine/engine.go` to understand the exact current flow before modifying.

- [ ] **Step 2: Modify Apply() to return BlueprintResult**

The `Apply()` method signature changes from returning `(*ApplyResult, error)` to `(*BlueprintResult, error)`. The method generates the manifest as before but calls `proposeBlueprint()` instead of `applyManifest()` at the final step.

Key changes to `Apply()`:
- Return type: `*BlueprintResult` instead of `*ApplyResult`
- After LLM returns manifest, call `e.proposeBlueprint(newManifest)` instead of `e.applyManifest()`
- Emit `EventBlueprintProposed` with the blueprint
- For conversational responses (ChatOnlyError), return `BlueprintResult{ChatResponse: text}`
- For multi-step plans: execute all steps to build the final manifest, then propose the combined result

Note: The existing `applyManifest()` method is NOT removed — it's called by `ApproveBlueprint()`.

- [ ] **Step 3: Update TUI app.go to handle BlueprintResult**

In `internal/tui/app.go`, the `applyPrompt()` method currently returns `ApplyResultMsg`. Change it to call `eng.Apply()` and return either `BlueprintProposedMsg` (if blueprint proposed) or `ApplyResultMsg` (if chat response / error):

Update the `applyPrompt` method:

```go
func (m RootModel) applyPrompt(prompt string) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = ApplyResultMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		result, err := eng.Apply(ctx, prompt)
		if err != nil {
			return ApplyResultMsg{Err: err}
		}
		if result.Blueprint != nil {
			return BlueprintProposedMsg{Blueprint: result.Blueprint}
		}
		if result.ChatResponse != "" {
			return ApplyResultMsg{Result: &engine.ApplyResult{ChatResponse: result.ChatResponse}}
		}
		return ApplyResultMsg{Err: fmt.Errorf("unexpected empty result")}
	}
}
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./...`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/engine/engine.go internal/tui/app.go
git commit -m "feat(blueprint): modify Apply() to propose blueprint instead of direct apply"
```

---

### Task 5: System Prompt Enhancement

**Files:**
- Modify: `internal/llm/provider.go`

- [ ] **Step 1: Append Architectural Heuristics to BuildSystemPrompt()**

At the end of `BuildSystemPrompt()`, before the final return, add the Architectural Heuristics section:

```go
	b.WriteString("\n## Architectural Heuristics\n\n")
	b.WriteString("When designing an API, think beyond simple CRUD. For every request, consider:\n\n")
	b.WriteString("1. STATE TRANSITIONS: If an entity has a lifecycle (draft→active→closed),\n")
	b.WriteString("   create explicit action endpoints (POST /resource/:id/activate) instead\n")
	b.WriteString("   of generic PUT with a status field.\n\n")
	b.WriteString("2. COMPUTED ENDPOINTS: If users need aggregated/calculated data, create\n")
	b.WriteString("   dedicated endpoints with the computation in the script, not raw SELECTs.\n\n")
	b.WriteString("3. VALIDATION GUARDS: Add business rule checks before mutations. Check\n")
	b.WriteString("   time limits, prevent duplicates, verify prerequisites.\n\n")
	b.WriteString("4. LIFECYCLE HOOKS: When one action should trigger updates elsewhere,\n")
	b.WriteString("   include that logic. Submitting a quiz should update the student's stats.\n\n")
	b.WriteString("Your blueprint MUST include at least 2 routes that go beyond basic CRUD.\n")
```

- [ ] **Step 2: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/llm/provider.go
git commit -m "feat(blueprint): add Architectural Heuristics to LLM system prompt"
```

---

### Task 6: TUI Proposal State + Conversation Handling

**Files:**
- Modify: `internal/tui/conversation.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Add proposalPending state to ConversationModel**

In `internal/tui/conversation.go`, add to the struct:

```go
type ConversationModel struct {
	messages        []Message
	input           string
	cursorPos       int
	width           int
	height          int
	focused         bool
	scrollOffset    int
	proposalPending bool // true when blueprint is awaiting approval
}
```

- [ ] **Step 2: Modify input handling for proposal state**

In the `Update()` method's `"enter"` case (conversation.go), add proposal handling before the default case. After the `/help` case and before `default:`:

```go
			default:
				if m.proposalPending {
					lower := strings.ToLower(trimmed)
					switch {
					case lower == "y" || lower == "yes":
						return m, func() tea.Msg { return BlueprintApproveMsg{} }
					case lower == "n" || lower == "no" || lower == "/cancel":
						m.proposalPending = false
						m.AddMessage(Message{Role: RoleSystem, Content: "Blueprint cancelled."})
						return m, func() tea.Msg { return BlueprintCancelMsg{} }
					case lower == "enhance":
						m.AddMessage(Message{Role: RoleUser, Content: trimmed})
						m.AddMessage(Message{Role: RoleSystem, Content: "Enhancing blueprint..."})
						return m, func() tea.Msg {
							return BlueprintRefineMsg{Feedback: "The current design is too CRUD-heavy. Add state transitions for entities with lifecycle, computed endpoints for analytics, or validation guards for business rules."}
						}
					default:
						// User typed feedback for refinement
						m.AddMessage(Message{Role: RoleUser, Content: trimmed})
						m.AddMessage(Message{Role: RoleSystem, Content: "Refining blueprint..."})
						return m, func() tea.Msg {
							return BlueprintRefineMsg{Feedback: trimmed}
						}
					}
				}
				// Send as prompt to AI
				return m, func() tea.Msg { return SubmitPromptMsg(trimmed) }
```

- [ ] **Step 3: Change prompt indicator when proposal is pending**

In `renderInput()` method, change the prompt:

```go
func (m ConversationModel) renderInput() string {
	promptText := "vibe> "
	if m.proposalPending {
		promptText = "blueprint> "
	}
	prompt := stylePrompt.Render(promptText)
```

- [ ] **Step 4: Handle BlueprintProposedMsg in app.go Update()**

In `internal/tui/app.go`, add a new case in `Update()` after the `ApplyResultMsg` case:

```go
	case BlueprintProposedMsg:
		m.conversation.RemoveLastSystem()
		m.conversation.proposalPending = true

		summary := engine.FormatBlueprintSummary(msg.Blueprint)
		m.conversation.AddMessage(Message{
			Role:    RoleAssistant,
			Content: summary,
		})

		// Show blueprint URL
		m.conversation.AddMessage(Message{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Full blueprint: %s/_blueprint", m.serverURL),
		})

		m.conversation.AddMessage(Message{
			Role:    RoleSystem,
			Content: "[y] approve  |  type feedback to refine  |  [n] cancel",
		})

	case BlueprintApproveMsg:
		m.conversation.proposalPending = false
		m.conversation.AddMessage(Message{Role: RoleSystem, Content: "Applying blueprint..."})
		cmd := m.approveBlueprint()
		cmds = append(cmds, cmd)

	case BlueprintRefineMsg:
		cmd := m.refineBlueprint(msg.Feedback)
		cmds = append(cmds, cmd)

	case BlueprintCancelMsg:
		if m.engine != nil {
			m.engine.CancelBlueprint()
		}

	case BlueprintAppliedMsg:
		m.conversation.RemoveLastSystem()
		if msg.Err != nil {
			m.conversation.AddMessage(Message{
				Role:    RoleError,
				Content: fmt.Sprintf("Error: %v", msg.Err),
			})
		} else {
			summary := engine.FormatChangeSummary(msg.Result)
			m.conversation.AddMessage(Message{
				Role:    RoleAssistant,
				Content: summary,
			})
			if msg.Result != nil && msg.Result.Manifest != nil {
				m.dashboard.UpdateFromManifest(msg.Result.Manifest)
			}
		}
```

- [ ] **Step 5: Add approveBlueprint and refineBlueprint helper methods**

Add to `internal/tui/app.go` after the `applyUndo()` method:

```go
// approveBlueprint dispatches engine.ApproveBlueprint as a tea.Cmd.
func (m RootModel) approveBlueprint() tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = BlueprintAppliedMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		result, err := eng.ApproveBlueprint(ctx)
		return BlueprintAppliedMsg{Result: result, Err: err}
	}
}

// refineBlueprint dispatches engine.RefineBlueprint as a tea.Cmd.
func (m RootModel) refineBlueprint(feedback string) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = ApplyResultMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		bp, err := eng.RefineBlueprint(ctx, feedback)
		if err != nil {
			return ApplyResultMsg{Err: err}
		}
		return BlueprintProposedMsg{Blueprint: bp}
	}
}
```

- [ ] **Step 6: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./...`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/tui/conversation.go internal/tui/app.go
git commit -m "feat(blueprint): add TUI proposal state — approve, refine, cancel flow"
```

---

### Task 7: Bridge + Event Wiring

**Files:**
- Modify: `internal/tui/bridge.go`

- [ ] **Step 1: Add blueprint event subscriptions**

In `internal/tui/bridge.go`, in `NewBridge()`, after the existing `EventStepCompleted` subscription, add:

```go
	bus.Subscribe(engine.EventBlueprintProposed, func(e engine.Event) {
		if bp, ok := e.Data.(engine.BlueprintInfo); ok {
			program.Send(BlueprintProposedMsg{Blueprint: &bp})
		}
	})

	bus.Subscribe(engine.EventBlueprintRefined, func(e engine.Event) {
		if bp, ok := e.Data.(engine.BlueprintInfo); ok {
			program.Send(BlueprintProposedMsg{Blueprint: &bp})
		}
	})
```

- [ ] **Step 2: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/tui/bridge.go
git commit -m "feat(blueprint): wire blueprint events to TUI via bridge"
```

---

### Task 8: Web Blueprint API + HTML Page

**Files:**
- Create: `internal/web/blueprint.go`
- Create: `internal/web/static/blueprint.html`
- Modify: `internal/web/console.go`
- Modify: `internal/web/mux.go`

- [ ] **Step 1: Create blueprint API handler**

```go
// internal/web/blueprint.go
package web

import (
	"encoding/json"
	"net/http"

	"github.com/vibeserve/vibeserve/internal/engine"
)

// BlueprintHandler serves the blueprint API and preview page.
type BlueprintHandler struct {
	engine *engine.Engine
}

// NewBlueprintHandler creates a BlueprintHandler.
func NewBlueprintHandler(eng *engine.Engine) *BlueprintHandler {
	return &BlueprintHandler{engine: eng}
}

type blueprintResponse struct {
	Status     string           `json:"status"`
	Manifest   any              `json:"manifest,omitempty"`
	Changes    []changeInfo     `json:"changes,omitempty"`
	Heuristics *heuristicInfo   `json:"heuristics,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
}

type changeInfo struct {
	Type     string `json:"type"`
	Detail   string `json:"detail"`
	Breaking bool   `json:"breaking"`
}

type heuristicInfo struct {
	Score       int      `json:"score"`
	Hints       []string `json:"hints"`
	Suggestions []string `json:"suggestions"`
}

// HandleBlueprint serves GET /_api/blueprint.
func (bh *BlueprintHandler) HandleBlueprint(w http.ResponseWriter, r *http.Request) {
	bp := bh.engine.PendingBlueprint()
	if bp == nil {
		writeConsoleJSON(w, http.StatusOK, blueprintResponse{Status: "none"})
		return
	}

	var changes []changeInfo
	for _, c := range bp.Changes {
		breaking := c.Type == "DROP_COLUMN" || c.Type == "REMOVE_ROUTE"
		changes = append(changes, changeInfo{
			Type:     string(c.Type),
			Detail:   c.Detail,
			Breaking: breaking,
		})
	}

	resp := blueprintResponse{
		Status:   "pending",
		Manifest: bp.Manifest,
		Changes:  changes,
		Heuristics: &heuristicInfo{
			Score:       bp.Heuristics.Score,
			Hints:       bp.Heuristics.Hints,
			Suggestions: bp.Heuristics.Suggestions,
		},
		Warnings: bp.Warnings,
	}

	writeConsoleJSON(w, http.StatusOK, resp)
}
```

- [ ] **Step 2: Create blueprint.html preview page**

Create `internal/web/static/blueprint.html` — a self-contained HTML page that fetches `/_api/blueprint` and renders it as a styled read-only document. The page should:
- Poll `/_api/blueprint` every 2 seconds
- Show "No pending blueprint" when status is "none"
- Render manifest name, description, heuristic score badge
- Module view: tables grouped with their routes
- Route table with method, path, description
- Schema view with columns and types
- Script preview with Tengo code
- Heuristic highlights (✦ items)
- Breaking change warnings (⚠ section)
- Footer: "Return to your terminal to approve or refine this blueprint."
- Dark theme matching the console page

- [ ] **Step 3: Register blueprint routes in mux**

In `internal/web/mux.go`, update `NewConsoleMux` to accept a `*BlueprintHandler` parameter and register routes:

```go
func NewConsoleMux(apiHandler http.Handler, console *Console, wsHub *WSHub, blueprint *BlueprintHandler) http.Handler {
```

Add before the `mux.Handle("/", apiHandler)` line:

```go
	mux.HandleFunc("GET /_api/blueprint", blueprint.HandleBlueprint)
	mux.Handle("GET /_blueprint", http.StripPrefix("/_blueprint", http.FileServer(http.FS(blueprintFS))))
```

For the blueprint page, serve it from the same embedded FS. The simplest approach: add a handler that serves `blueprint.html` from the static FS:

```go
	mux.HandleFunc("GET /_blueprint", func(w http.ResponseWriter, r *http.Request) {
		data, err := StaticFS.ReadFile("static/blueprint.html")
		if err != nil {
			http.Error(w, "blueprint page not found", 404)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
```

- [ ] **Step 4: Update cmd/vibeserve/main.go to wire BlueprintHandler**

In both `runDev()` and `runUp()`, create the BlueprintHandler and pass it to `NewConsoleMux`:

```go
	blueprintHandler := web.NewBlueprintHandler(eng)
	mux := web.NewConsoleMux(apiHandler, consoleHandler, wsHub, blueprintHandler)
```

- [ ] **Step 5: Verify build**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./...`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add internal/web/blueprint.go internal/web/static/blueprint.html internal/web/mux.go cmd/vibeserve/main.go
git commit -m "feat(blueprint): add web blueprint API + preview page at /_blueprint"
```

---

### Task 9: Full Build + Test

- [ ] **Step 1: Build the full binary**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go build ./cmd/vibeserve`
Expected: Build succeeds

- [ ] **Step 2: Run all tests**

Run: `cd /Users/kent/Documents/Projects/vibeserve && go test ./... -v`
Expected: All packages pass

- [ ] **Step 3: Verify help output**

Run: `cd /Users/kent/Documents/Projects/vibeserve && ./vibeserve --help`
Expected: Shows available commands including export

- [ ] **Step 4: Commit final state**

```bash
cd /Users/kent/Documents/Projects/vibeserve
git add -A
git commit -m "feat: Proposal Mode complete — blueprint review before apply"
```
