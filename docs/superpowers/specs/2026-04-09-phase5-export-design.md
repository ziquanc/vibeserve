# Phase 5: The Exit — `vibeserve export`

## Goal

Graduate a VibeServe prototype into a standalone, production-ready Go server. The command reads the current manifest and generates a complete Go project with Chi routing, sqlx database access, typed models, and a clean repository pattern — no VibeServe runtime dependency, no Tengo.

## Architecture

`vibeserve export [dir]` runs a deterministic 7-stage pipeline that transforms the manifest into a Go project. Tengo scripts are translated to native Go handlers via a line-oriented pattern matcher. An optional `--ai` flag invokes the configured LLM for complex logic the matcher can't handle.

The generated project follows idiomatic Go conventions: `cmd/api/main.go` entry point, `internal/` packages for model/repository/handler, struct-based dependency injection, and a `Store` interface for database abstraction.

## Tech Stack (Generated Project)

- **Router:** `go-chi/chi/v5`
- **Database:** `jmoiron/sqlx` + `modernc.org/sqlite` (pure Go, no CGO)
- **No external frameworks** — stdlib `net/http`, `encoding/json`, `context`

---

## CLI Interface

### Command Signature

```
vibeserve export [output-dir] [flags]
```

### Flags

| Flag | Description |
|---|---|
| `--force` | Overwrite existing directory without prompting |
| `--ai` | Use configured LLM to translate complex Tengo logic |
| `-m, --manifest` | Manifest path (default: `.vibe/manifest.json`) |

### Behavior

1. **Smart default naming:** If no `output-dir` given, derive from manifest name: `vibe-export-<slugified-name>`.
2. **Interactive confirm:** If directory exists and no `--force`, prompt: `Directory already exists. Overwrite? [y/N]`.
3. Run the 7-stage pipeline.
4. **Success banner:**

```
✓ Export complete!

  Your production Go server is ready at: ./my-api

  To start:
    cd my-api
    go run ./cmd/api

  Check README.md for API documentation.
```

---

## Generated Project Structure

```
<output-dir>/
├── cmd/
│   └── api/
│       └── main.go              # Chi router, middlewares, graceful shutdown
├── internal/
│   ├── model/
│   │   └── models.go            # Go structs from manifest schemas
│   ├── repository/
│   │   ├── store.go             # Store interface (usage-driven methods)
│   │   └── sqlite.go            # SQLite implementation via sqlx
│   └── handler/
│       └── handlers.go          # HTTP handlers (one per route)
├── go.mod                       # Module + dependencies
├── Dockerfile                   # Multi-stage build
├── openapi.yaml                 # OpenAPI 3.x spec
├── README.md                    # Auto-generated API docs
└── state.db                     # Copied from .vibe/state.db (if exists)
```

---

## Export Pipeline

### Stage 1: Load Manifest

Read `.vibe/manifest.json` (or path from `--manifest` flag). Abort with clear error if missing or empty.

### Stage 2: Validate

Run `manifest.Validate()` (structural, referential, compilation checks). Abort if invalid — do not export broken APIs. Display validation errors to the user.

### Stage 3: Generate Models

Map each manifest schema to a Go struct in `internal/model/models.go`.

**Type mapping:**

| Manifest Type | Go Type |
|---|---|
| `INTEGER` + primary + auto | `int64` |
| `INTEGER` | `int64` |
| `TEXT` | `string` |
| `REAL` | `float64` |
| `BOOLEAN` | `bool` |
| `DATE` | `time.Time` |
| `DATETIME` | `time.Time` |

Each struct field gets `db:"column_name"` and `json:"column_name"` tags. Table names are converted to PascalCase singular for the struct name (e.g., `vehicles` → `Vehicle`).

Example output:
```go
type Vehicle struct {
    ID    int64   `db:"id" json:"id"`
    Make  string  `db:"make" json:"make"`
    Plate string  `db:"plate" json:"plate"`
    Rate  float64 `db:"rate" json:"rate"`
}
```

### Stage 4: Generate Repository

Two files in `internal/repository/`:

**`store.go`** — Store interface with methods derived from what routes actually use (usage-driven, no dead code):

```go
type Store interface {
    ListVehicles(ctx context.Context) ([]model.Vehicle, error)
    GetVehicle(ctx context.Context, id int64) (*model.Vehicle, error)
    CreateVehicle(ctx context.Context, v *model.Vehicle) error
    // Only methods that routes reference are generated
}
```

**`sqlite.go`** — SQLite implementation using sqlx:
- Connection string defaults to `state.db?_pragma=foreign_keys(1)` to preserve manifest referential integrity.
- `NewSQLiteStore(dsn string)` constructor.

### Stage 5: Generate Handlers

**Handler struct with injected Store:**
```go
type Handler struct {
    store repository.Store
}

func NewHandler(store repository.Store) *Handler {
    return &Handler{store: store}
}
```

Each manifest route becomes a method on Handler. Tengo scripts are translated via the pattern matcher.

**Pattern matcher coverage:**

| Tengo Pattern | Go Translation |
|---|---|
| `db.query(sql, params)` | `h.store.DB().SelectContext(ctx, &items, sql, params...)` |
| `db.query_one(sql, params)` | `h.store.DB().GetContext(ctx, &item, sql, params...)` |
| `db.insert(table, data)` | `h.store.CreateX(ctx, &input)` |
| `db.update(table, id, data)` | `h.store.UpdateX(ctx, id, &input)` |
| `db.delete(table, id)` | `h.store.DeleteX(ctx, id)` |
| `request.param(name)` | `chi.URLParam(r, name)` + int64 parse if numeric |
| `request.query(name)` | `r.URL.Query().Get(name)` |
| `request.body()` | `json.NewDecoder(r.Body).Decode(&input)` |
| `response.json(data)` | `render.JSON(w, r, data)` |
| `response.json(data, status)` | `w.WriteHeader(status); render.JSON(w, r, data)` |
| `response.fail(status, msg)` | `http.Error(w, msg, status); return` |
| `if`/`else` blocks | Preserved as Go `if`/`else` |
| Variable assignments | Translated with inferred types |

**Patterns that fall through to TODO stubs:**
- `date.now()`, `date.diff_days()` — varied usage patterns
- `crypto.sha256()`, `crypto.md5()` — security-sensitive, best reviewed manually
- Complex control flow (nested loops, multi-step computations)
- String manipulation beyond simple concatenation

Untranslated patterns produce:
```go
// TODO: Manual implementation required for Tengo logic:
//   result := date.diff_days(start, end)
```

**`--ai` mode:** After template-based generation, any handler with TODO stubs is sent to the configured LLM with context (original Tengo script, Go model structs, Store interface). The LLM response replaces the TODO stub. If LLM fails, the TODO remains (graceful degradation).

### Stage 6: Generate Scaffold

**`cmd/api/main.go`:**
- Chi router with default middlewares: `middleware.RequestID`, `middleware.RealIP`, `middleware.Logger`, `middleware.Recoverer`
- Route registration wiring all generated handlers
- Configurable port via `PORT` env var or `-p` flag, default `8080`
- Graceful shutdown on SIGINT/SIGTERM

**`go.mod`:**
- Module name: slugified manifest name (e.g., manifest name "Car Rental API" → `module car-rental-api`)
- Dependencies: chi, sqlx, modernc.org/sqlite

**`Dockerfile`:**
- Multi-stage build: Go builder stage → minimal final image (alpine or scratch)
- Copies binary + `state.db`
- Exposes port, sets entrypoint

**`README.md`:**
- Project name and description from manifest
- Route table (method, path, description)
- Setup instructions (`go run`, Docker, environment variables)
- Schema documentation (tables, columns, types)

**`state.db`:**
- Copied from `.vibe/state.db` if it exists (preserves seed data and user data)

### Stage 7: Post-process

- Run `go mod tidy` in the output directory to resolve all dependencies
- Run `gofmt` on all generated `.go` files
- Print the success banner with next-step instructions

---

## OpenAPI Spec Generation

In addition to the Go project, generate an `openapi.yaml` file in the output directory.

**Mapping:**
- Manifest `name` / `description` → OpenAPI `info.title` / `info.description`
- Each manifest route → OpenAPI path + operation
- Route `request_body` → OpenAPI `requestBody` schema
- Route `response_type` ("array" / "object") → OpenAPI response schema referencing the model
- Manifest schemas → OpenAPI `components/schemas` (reusable type definitions)
- Path parameters (`:id`) → OpenAPI path parameters (`{id}`)

The OpenAPI spec is generated deterministically from the manifest — no LLM involved.

---

## Code Location in VibeServe

New package: `internal/export/`

| File | Responsibility |
|---|---|
| `exporter.go` | Pipeline orchestrator — runs all 7 stages |
| `models.go` | Stage 3: manifest schema → Go struct generation |
| `repository.go` | Stage 4: Store interface + SQLite impl generation |
| `handlers.go` | Stage 5: handler generation + pattern matcher |
| `scaffold.go` | Stage 6: main.go, go.mod, Dockerfile, README |
| `openapi.go` | OpenAPI 3.x YAML generation |
| `templates/` | Go `text/template` files for each generated file |

---

## Error Handling

- **No manifest:** `Error: no manifest found at .vibe/manifest.json. Run 'vibeserve' first to create your API.`
- **Invalid manifest:** `Error: manifest validation failed: <details>. Fix issues before exporting.`
- **Directory exists (no --force):** Interactive prompt, abort on "N"
- **go mod tidy fails:** Warning (non-fatal) — generated code is still written
- **`--ai` LLM failure:** Warning per handler — TODO stubs remain, export continues

---

## Scope Boundaries

**In scope:**
- Full Go project generation from manifest
- Template-based Tengo→Go translation
- OpenAPI 3.x spec generation
- `--ai` flag for LLM-assisted translation
- SQLite Store implementation with foreign keys enabled
- Dockerfile and README generation
- Data copy (state.db)

**Out of scope (future work):**
- PostgreSQL Store implementation (interface exists for user to add)
- Client SDK generation from OpenAPI
- Incremental re-export (always full regeneration)
- Test file generation for handlers
