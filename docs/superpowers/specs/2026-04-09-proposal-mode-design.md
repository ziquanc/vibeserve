# Proposal Mode — Blueprint Review Before Apply

## Goal

Transform VibeServe from a blind code generator into an architectural collaborator. When a user submits a prompt, the engine generates a blueprint (proposed manifest + architectural analysis) and pauses for review. The user approves, refines, or cancels before any changes touch the database or routes.

## Architecture

The engine's `Apply()` method gains a two-phase flow: **propose** then **apply**. Phase 1 generates a manifest, validates it, diffs it, scores it with architectural heuristics, and emits a `BlueprintProposed` event. Phase 2 only runs when the user explicitly approves — it applies migrations, updates routes, seeds data.

A read-only web page at `/_blueprint` renders the pending blueprint as a styled document. All control (approve/refine/cancel) stays in the terminal input field.

## Tech Stack

- No new dependencies
- Reuses existing `web/` embed pattern, mux routing, WebSocket hub
- Heuristic scoring is deterministic pattern matching (no LLM call)

---

## Event Flow — The Confirmation Loop

### Current Flow (Direct Apply)

```
User prompt → LLM → Manifest → validate → diff → snapshot → migrate → done
```

### New Flow (Proposal Mode)

```
User prompt → LLM → Manifest → validate → diff → heuristics → [PAUSE]
                                                                   ↓
                                                        user reviews blueprint
                                                                   ↓
                                                   approve / refine / cancel
                                                        ↓           ↓
                                                    apply      LLM re-generate
                                                                   ↓
                                                            new blueprint → [PAUSE]
```

### New Events

| Event | Data Type | When Emitted |
|---|---|---|
| `EventBlueprintProposed` | `BlueprintInfo` | Engine generates manifest, computes diff + heuristics, pauses |
| `EventBlueprintRefined` | `BlueprintInfo` | User gave feedback, LLM regenerated, new blueprint ready |
| `EventBlueprintApproved` | `BlueprintInfo` | User typed `y`, engine proceeds to apply |

### BlueprintInfo Struct

```go
type BlueprintInfo struct {
    Manifest   *manifest.Manifest
    Changes    []manifest.Change
    Summary    string           // human-readable change summary
    Heuristics HeuristicResult  // architectural quality analysis
}
```

### New TUI Messages

| Message | Direction | Purpose |
|---|---|---|
| `BlueprintProposedMsg` | Engine → TUI | Carries blueprint for display |
| `BlueprintApproveMsg` | TUI → Engine | User confirmed with `y` |
| `BlueprintRefineMsg` | TUI → Engine | User typed feedback or `enhance` |
| `BlueprintCancelMsg` | TUI → Engine | User typed `n` — discard blueprint |

---

## Engine Changes

### Engine State

New field on `Engine` struct:

```go
type Engine struct {
    // ... existing fields ...
    pendingBlueprint *BlueprintInfo  // set when proposed, cleared on approve/cancel
}
```

### Modified Apply Flow

`Apply(ctx, prompt)` changes behavior:

1. Planning phase — unchanged (LLM breaks request into steps)
2. For each step, LLM generates a manifest
3. `repairManifest()` — unchanged
4. `Validate()` — unchanged
5. `Diff()` — compute changes against current manifest
6. **NEW:** `scoreHeuristics(manifest, changes)` — compute architectural quality
7. **NEW:** Store as `pendingBlueprint`, emit `EventBlueprintProposed`
8. **NEW:** Return `BlueprintResult` instead of `ApplyResult`

The old `applyManifest()` (snapshot, migrate, routes, scripts, seeds, save) is extracted into a separate method called only on approval.

### New Engine Methods

```go
// ApproveBlueprint applies the pending blueprint.
func (e *Engine) ApproveBlueprint(ctx context.Context) (*ApplyResult, error)

// RefineBlueprint sends feedback to LLM, generates new blueprint.
func (e *Engine) RefineBlueprint(ctx context.Context, feedback string) (*BlueprintInfo, error)

// CancelBlueprint discards the pending blueprint.
func (e *Engine) CancelBlueprint()

// HasPendingBlueprint returns true if a blueprint is awaiting approval.
func (e *Engine) HasPendingBlueprint() bool

// PendingBlueprint returns the current pending blueprint (for web API).
func (e *Engine) PendingBlueprint() *BlueprintInfo
```

### BlueprintResult (replaces ApplyResult as return type of Apply)

```go
type BlueprintResult struct {
    Blueprint    *BlueprintInfo // non-nil when proposal is pending
    ChatResponse string         // non-empty if LLM responded conversationally
}
```

The existing `ApplyResult` (with `Changes`, `Warnings`, `Manifest`) is now returned only by `ApproveBlueprint()`. The TUI handles `BlueprintResultMsg` in place of the current `ApplyResultMsg` for the initial prompt, and receives `ApplyResultMsg` only after approval.

---

## Architectural Heuristics

### Purpose

Ensure the LLM generates architecturally thoughtful APIs, not just CRUD scaffolds. The heuristic system works at two levels:

1. **Prompt-level** — system prompt instructs the LLM to think beyond CRUD
2. **Engine-level** — pattern matching scores the output and suggests improvements

### Heuristic Categories

| Category | Detection Pattern | Example |
|---|---|---|
| **State Transitions** | Routes with action verbs on a resource (`:id/start`, `:id/submit`, `:id/activate`) | `POST /exams/:id/start`, `PATCH /exams/:id/submit` |
| **Computed Aggregations** | Scripts containing `COUNT`, `AVG`, `SUM`, `GROUP BY` in SQL queries | `GET /students/:id/progress` |
| **Validation Guards** | Scripts with `response.fail()` before the primary db operation | Time limit check before accepting exam submission |
| **Lifecycle Hooks** | Scripts that call `db.update`/`db.insert` on a table different from the route's primary resource | Submitting quiz updates both `submissions` and `student_stats` |

### HeuristicResult Struct

```go
type HeuristicResult struct {
    Score       int      // 0-10
    Hints       []string // what the LLM proposed beyond CRUD (shown as "✦" items)
    Suggestions []string // what could still be added
}
```

### Scoring Logic

`scoreHeuristics(manifest, changes)` scans routes and scripts:

- +2 points per state transition route detected
- +2 points per computed aggregation endpoint
- +1 point per validation guard
- +1 point per lifecycle hook
- Cap at 10

### Low-Score Behavior

If `Score < 4`:
- Add to `Suggestions`: "Architecture is mostly basic CRUD. Consider adding state transitions, computed endpoints, or validation guards."
- TUI renders a warning with the `enhance` option
- If user types `enhance`, engine calls `RefineBlueprint` with: "The current design is too CRUD-heavy. Add state transitions for entities with lifecycle, computed endpoints for analytics, or validation guards for business rules."

### System Prompt Addition

Appended to `BuildSystemPrompt()`:

```
## Architectural Heuristics

When designing an API, think beyond simple CRUD. For every request, consider:

1. STATE TRANSITIONS: If an entity has a lifecycle (draft→active→closed),
   create explicit action endpoints (POST /resource/:id/activate) instead
   of generic PUT with a status field.

2. COMPUTED ENDPOINTS: If users need aggregated/calculated data, create
   dedicated endpoints with the computation in the script, not raw SELECTs.

3. VALIDATION GUARDS: Add business rule checks before mutations. Check
   time limits, prevent duplicates, verify prerequisites.

4. LIFECYCLE HOOKS: When one action should trigger updates elsewhere,
   include that logic. Submitting a quiz should update the student's stats.

Your blueprint MUST include at least 2 routes that go beyond basic CRUD.
Mark these routes with a [HEURISTIC: category] comment in the description.
```

---

## Breaking Change Detection

When `Diff()` computes changes, flag destructive operations:

| Change Type | Breaking? | Warning |
|---|---|---|
| `DROP_COLUMN` | Yes | "Column `X` will be removed from table `Y`" |
| `REMOVE_ROUTE` | Yes | "Endpoint `METHOD /path` will be removed" |
| `REMOVE_SCRIPT` | Soft | "Script `X` will be removed" |
| `ADD_TABLE`, `ADD_COLUMN`, `ADD_ROUTE` | No | Normal addition |
| `UPDATE_ROUTE`, `UPDATE_SCRIPT` | Soft | "Existing behavior will change" |

Breaking changes are:
- Highlighted with `⚠` in TUI output
- Shown in a dedicated "Breaking Changes" section in the web blueprint
- Included in the `BlueprintInfo` as a `Warnings []string` field (reuses existing warning pattern)

---

## TUI Interaction

### Proposal State

`ConversationModel` gains a new field:

```go
type ConversationModel struct {
    // ... existing fields ...
    proposalPending bool
}
```

When `proposalPending` is true, the input prompt changes from `vibe>` to `blueprint>` and input is routed to proposal handling:

| User Input | Action |
|---|---|
| `y` or `yes` | Emit `BlueprintApproveMsg` |
| `n` or `no` or `/cancel` | Emit `BlueprintCancelMsg`, clear pending state |
| `enhance` | Emit `BlueprintRefineMsg` with auto-enhancement instruction |
| Any other text | Emit `BlueprintRefineMsg` with text as user feedback |

### TUI Rendering of Blueprint

When `BlueprintProposedMsg` arrives:

```
VibeServe: Blueprint ready (score: 7/10)

  Modules:
    Quiz Engine       — 4 routes (2 CRUD + 2 state transitions)
    Student Progress  — 2 routes (1 computed aggregation)
    Admin Dashboard   — 2 routes

  ✦ State machine: POST /quizzes/:id/start → PATCH /quizzes/:id/submit
  ✦ Auto-grading: submit triggers score calculation + progress update
  ✦ Time guard: submissions rejected past deadline

  8 routes, 3 tables, 8 scripts

  Full blueprint: http://localhost:8080/_blueprint

  [y] approve  |  type feedback to refine  |  [n] cancel
```

Low-score variant (score < 4):

```
VibeServe: Blueprint ready (score: 2/10)

  ⚠ Architecture is mostly basic CRUD. Consider:
    • State transitions for entities with lifecycle
    • Computed endpoints for analytics
    • Validation guards for business rules

  2 routes, 1 table, 2 scripts

  Full blueprint: http://localhost:8080/_blueprint

  [y] approve as-is  |  "enhance" to auto-improve  |  [n] cancel
```

Breaking change variant:

```
VibeServe: Blueprint ready (score: 6/10)

  ⚠ Breaking changes detected:
    • Column "old_field" will be removed from table "users"
    • Endpoint DELETE /legacy will be removed

  ...rest of blueprint...

  [y] approve (includes breaking changes)  |  [n] cancel
```

---

## Web Blueprint Preview

### Endpoint: `/_blueprint`

An embedded single-file HTML page (same pattern as `/_console/`) that renders the pending blueprint as a read-only styled document.

### API: `GET /_api/blueprint`

Returns the current blueprint state:

```json
{
  "status": "pending",
  "manifest": { ... },
  "changes": [
    {"type": "ADD_TABLE", "detail": "new table \"quizzes\"", "breaking": false},
    {"type": "ADD_ROUTE", "detail": "new route POST /quizzes/:id/start", "breaking": false}
  ],
  "heuristics": {
    "score": 7,
    "hints": [
      "State machine: POST /quizzes/:id/start → PATCH /quizzes/:id/submit",
      "Auto-grading: submit triggers score calculation"
    ],
    "suggestions": []
  },
  "warnings": []
}
```

When no blueprint is pending: `{"status": "none"}`.

### Web Page Layout

The HTML page renders:
1. **Header** — project name, blueprint status badge, heuristic score
2. **Module View** — tables grouped with their related routes and scripts
3. **Route Table** — method, path, description, heuristic tag if applicable
4. **Schema View** — table columns with types and constraints
5. **Script Preview** — Tengo code blocks with the business logic
6. **Heuristic Highlights** — the ✦ items prominently displayed
7. **Breaking Changes** — ⚠ section if any destructive diffs
8. **Change Summary** — what will be added/modified/removed

Auto-refresh: the page polls `/_api/blueprint` every 2 seconds and re-renders on change. Alternatively, connects to the existing `/_ws` WebSocket and listens for blueprint events.

### No Reverse Control

The web page has NO approve/reject buttons. It displays a footer: "Return to your terminal to approve or refine this blueprint."

---

## Code Location

### New files:

| File | Responsibility |
|---|---|
| `internal/engine/blueprint.go` | `BlueprintInfo`, `HeuristicResult`, `scoreHeuristics()`, `ApproveBlueprint()`, `RefineBlueprint()`, `CancelBlueprint()` |
| `internal/engine/blueprint_test.go` | Tests for heuristic scoring and blueprint lifecycle |
| `internal/engine/heuristics.go` | Heuristic pattern detection — scans routes/scripts for non-CRUD patterns |
| `internal/engine/heuristics_test.go` | Tests for each heuristic category |
| `internal/web/blueprint.go` | `GET /_api/blueprint` handler |
| `internal/web/static/blueprint.html` | Embedded blueprint preview page |

### Modified files:

| File | Change |
|---|---|
| `internal/engine/engine.go` | Add `pendingBlueprint` field, modify `Apply()` to return `BlueprintResult`, extract `commitBlueprint()` from `applyManifest()` |
| `internal/engine/events.go` | Add `EventBlueprintProposed`, `EventBlueprintRefined`, `EventBlueprintApproved` |
| `internal/llm/provider.go` | Append Architectural Heuristics section to `BuildSystemPrompt()` |
| `internal/tui/messages.go` | Add `BlueprintProposedMsg`, `BlueprintApproveMsg`, `BlueprintRefineMsg`, `BlueprintCancelMsg` |
| `internal/tui/app.go` | Handle new message types in `Update()`, manage proposal state |
| `internal/tui/conversation.go` | Add `proposalPending` state, `blueprint>` prompt, route input to proposal actions |
| `internal/web/mux.go` | Register `/_blueprint` and `/_api/blueprint` routes |
| `internal/web/embed.go` | Embed `blueprint.html` |
| `internal/tui/bridge.go` | Subscribe to new blueprint events |

---

## Scope Boundaries

**In scope:**
- Two-phase apply flow (propose → approve)
- Architectural heuristic scoring (pattern matching, no LLM)
- Blueprint refinement loop (user feedback → LLM regeneration)
- Low-score auto-suggestion ("enhance" option)
- Breaking change detection and warnings
- Read-only web preview at `/_blueprint`
- System prompt enhancement with Architectural Heuristics section

**Out of scope (future work):**
- Domain Awareness packs (industry-specific prompt templates for education, e-commerce, finance)
- Blueprint editing in the web UI (editing stays in terminal)
- Blueprint history/versioning
- Automatic approval for trivial changes (always requires explicit `y`)
