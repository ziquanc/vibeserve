# State Machines — Design Spec

## Goal

Add state machine definitions to the manifest so entities with lifecycles (orders, tickets, articles) have enforced transitions with guards. Auto-generate transition routes from the definition.

## Manifest Schema

Add optional `state_machine` field to `Schema`:

```go
type StateMachine struct {
    Field       string       `json:"field"`       // column name (e.g. "status")
    Initial     string       `json:"initial"`     // initial state value
    Transitions []Transition `json:"transitions"`
}

type Transition struct {
    From      string          `json:"from"`      // source state
    To        string          `json:"to"`        // target state
    Action    string          `json:"action"`    // verb (e.g. "approve") → becomes POST /table/:id/action
    Guard     *TransitionGuard `json:"guard,omitempty"`
}

type TransitionGuard struct {
    Role      string `json:"role,omitempty"`      // required user role
    Condition string `json:"condition,omitempty"` // field condition (e.g. "total > 0")
}
```

Example manifest:
```json
{
  "table": "orders",
  "columns": [...],
  "state_machine": {
    "field": "status",
    "initial": "draft",
    "transitions": [
      {"from": "draft", "to": "submitted", "action": "submit", "guard": {"condition": "total > 0"}},
      {"from": "submitted", "to": "approved", "action": "approve", "guard": {"role": "admin"}},
      {"from": "submitted", "to": "rejected", "action": "reject", "guard": {"role": "admin"}},
      {"from": "approved", "to": "shipped", "action": "ship"},
      {"from": "shipped", "to": "delivered", "action": "deliver"}
    ]
  }
}
```

## Auto-Generated Routes

For each transition, generate `POST /{table}/:id/{action}` with a script that:

1. Fetches the row by id (404 if not found or soft-deleted)
2. Checks current state matches `from` (400 if wrong state)
3. If guard has `role` — checks `request.auth().role` (401/403)
4. If guard has `condition` — evaluates condition against current row (400 if not met)
5. Updates the status field to `to` + sets `updated_at`
6. Returns the updated row

Also generate `GET /{table}/:id/transitions` — returns the list of valid transitions for the current state.

## Runtime

- `state_machine` is read during manifest validation — check field exists in columns, all from/to states are consistent
- Default value for the status column is set to `initial`
- The transition routes are generated alongside CRUD routes in the engine
- State enforcement is in Tengo scripts (not DB constraints) — works with SQLite

## LLM Prompt

Add to the system prompt: when designing entities with lifecycles, generate a `state_machine` definition. Provide examples:
- "orders need approval" → draft/submitted/approved/rejected
- "articles should be published" → draft/review/published/archived
- "support tickets" → open/in_progress/resolved/closed

## Export

All three export formats (Go, Express, Next.js) generate transition routes with guards:
- Express: `router.post('/:id/approve', authenticate, (req, res) => { ... })`
- Go: handler with role middleware
- Next.js: API client gets `approveOrder(id)` function

The ER diagram shows states as a note on the entity.

## Validation

During manifest validation, check:
- `state_machine.field` exists in the schema's columns
- All `from` and `to` values are consistent (no orphan states)
- `action` names are unique per table
- `initial` state has at least one outgoing transition

## Out of Scope (v1.1)

- Hooks (side effects on transition)
- Parallel state machines on one table
- Timed transitions (auto-expire)
- State history/audit log table
