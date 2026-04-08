# VibeServe: Design Specification

> Single-binary CLI that generates stateful, logic-aware API backends from natural language.

## 1. Problem Statement

AI coding tools (Cursor, Bolt.new) solve UI generation but leave a "backend cliff." Developers choose between dead mock data and heavy infrastructure setup (Supabase/Docker/Postgres), breaking the vibe-coding flow. There is no tool that provides a zero-config, stateful, logic-aware backend that evolves through conversation.

## 2. Target Users

- Frontend engineers building UIs who need a real backend to code against
- Fast-moving fullstack developers prototyping before committing to infrastructure
- AI agent developers who need realistic API simulation environments

## 3. Core Value Proposition

VibeServe is a self-running virtual backend engine:

- **Zero-Config & Stateful**: Embedded SQLite with auto schema migration. Data survives restarts.
- **Logic Sandbox**: AI-generated business logic (discounts, permissions, state machines) runs in a sandboxed Tengo runtime.
- **Interactive Evolution**: `vibeserve dev` immerses the developer in a conversational loop where APIs hot-reload as they speak.
- **Exit Strategy**: One-click export to OpenAPI 3.x spec + idiomatic Go server. "Your launchpad, not your cage."

## 4. Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language | Go | Fast iteration, excellent stdlib, single-binary compilation, Bubble Tea ecosystem |
| LLM Strategy | Pluggable provider system | Vendor-agnostic; supports Claude, OpenAI, Ollama via config |
| Scripting Runtime | Tengo | Go-like syntax (AI-friendly), full control flow, safe sandboxing, custom object injection |
| Data Store | SQLite (modernc.org/sqlite, pure Go) | Zero-dependency, file-based persistence, trivial snapshotting via file copy |
| Architecture | Internal Message Bus (Approach B) | Event-driven module coordination; decoupled yet single-process |
| TUI Framework | Bubble Tea + Lip Gloss | Split-pane terminal UI with real-time dashboard |
| Export Format | OpenAPI 3.x + opinionated Go server (Chi) | Universal spec + batteries-included code output |
| Schema Evolution | Auto-migrate with snapshot-based undo | Preserves data continuity; SQLite file-copy snapshots are near-free |

## 5. Architecture

### 5.1 Module Boundaries

```
internal/
├── engine/          # Central coordinator, event bus, interface definitions
├── manifest/        # System Manifest types + differ (neutral, no deps)
├── llm/             # LLM provider adapters (produces manifests)
├── router/          # Trie-based dynamic HTTP routing
├── store/           # SQLite wrapper, migrations, snapshots, seeding
├── runtime/         # Tengo VM, Vibe Standard Library
├── tui/             # Bubble Tea split-pane UI
├── web/             # Embedded web console (observability + API explorer)
└── export/          # OpenAPI + Go server code generation
```

**Dependency rule**: No sibling package imports another sibling directly. All cross-module communication flows through the engine via interface injection or the event bus.

**Dependency graph** (acyclic):

```
              manifest (types only, no deps)
              ^   ^   ^   ^   ^
   llm   router  store  runtime  export
     \      \      |      /      /
      \---------- engine ------/
                  ^   ^
                tui   web
```

### 5.2 Interface Injection

The engine defines interfaces that decouple modules:

- `ScriptEvaluator` — implemented by `runtime`, consumed by `router`
- `DataStore` — implemented by `store`, consumed by `runtime/stdlib`
- `LLMProvider` — implemented by `llm/*`, consumed by `engine`
- `ManifestDiffer` — implemented by `manifest`, consumed by `engine`

At startup, `engine.New()` wires these together. Each module is testable in isolation with mock interfaces.

### 5.3 Event Bus

Typed, channel-based pub/sub within the single process. Not Kafka — just Go channels with a subscriber registry.

**Event Taxonomy:**

Manifest lifecycle:
- `ManifestGenerated` — LLM produced a new manifest
- `ManifestValidationFailed` — three-layer validation failed (structural, referential, or compilation)
- `ManifestDiffComputed` — Differ compared old vs new, carries `[]Change`

Schema / Data:
- `SchemaAltering` — about to alter, triggers snapshot
- `SchemaAltered` — ALTER TABLE applied
- `SchemaMigrationFailed` — migration failed, triggers rollback
- `SnapshotCreated` — `.vibe/snapshots/<id>` written
- `SnapshotRestored` — user invoked undo, DB rolled back
- `DataSeeded` — seed data inserted

Routes:
- `RouteAdded` — new path+method registered
- `RouteUpdated` — existing route's script changed
- `RouteRemoved` — route deregistered

Script / Runtime:
- `ScriptValidationStarted` — in-memory dry run begins
- `ScriptValidationPassed` — dry run succeeded
- `ScriptValidationFailed` — dry run failed, blocks RouteUpdated
- `ScriptLoaded` — Tengo script compiled and cached
- `ScriptExecuted` — request hit route, script ran
- `ScriptError` — runtime error during execution

Session:
- `SessionStarted` — `vibeserve dev` launched
- `UserPromptReceived` — user typed in TUI
- `LLMRequestStarted` — waiting for LLM
- `LLMRequestCompleted` — LLM responded

Observability:
- `HTTPRequestReceived` — incoming request
- `HTTPResponseSent` — outgoing response
- `LogEmitted` — general-purpose log

### 5.4 Event Flow Example

```
User: "add loyalty tiers to users, gold users get free shipping"
  |
  v
UserPromptReceived -> TUI: shows spinner
  |
  v
LLMRequestStarted -> LLM generates updated manifest
  |
  v
ManifestGenerated -> Differ runs
  |
  v
ManifestDiffComputed [SchemaChange, RouteChange, ScriptChange]
  |
  +--> SchemaAltering -> Store snapshots DB
  |       |
  |       v
  |    SchemaAltered -> TUI: "users.loyalty_tier added"
  |
  +--> ScriptValidationStarted -> in-memory dry run
  |       |
  |       v
  |    ScriptValidationPassed -> ScriptLoaded -> TUI: "logic compiled"
  |
  +--> RouteUpdated -> Trie re-registers handler -> TUI: "PUT /orders updated"
```

## 6. System Manifest Schema

The manifest is the "constitution" — the single source of truth produced by the LLM and consumed by all other modules.

```json
{
  "version": "1.0",
  "name": "project-name",
  "description": "Human-readable description",
  "schemas": [
    {
      "table": "table_name",
      "columns": [
        {
          "name": "column_name",
          "type": "INTEGER|TEXT|REAL|BOOLEAN|DATE|DATETIME",
          "primary": false,
          "auto": false,
          "required": false,
          "unique": false,
          "default": null,
          "references": null
        }
      ]
    }
  ],
  "routes": [
    {
      "path": "/resource/:id",
      "method": "GET|POST|PUT|PATCH|DELETE",
      "description": "What this endpoint does",
      "script": "script_name",
      "request_body": {},
      "response_type": "object|array"
    }
  ],
  "scripts": [
    {
      "name": "script_name",
      "code": "Tengo source using Vibe Standard Library"
    }
  ],
  "seeds": [
    {
      "table": "table_name",
      "rows": [{}]
    }
  ]
}
```

**LLM prompting rules:**
- The LLM always outputs a complete manifest (not a diff). The Differ handles change detection.
- The LLM receives the current manifest as context, plus the conversation history.
- Migration Memory Rule: the LLM prompt explicitly states "only add or modify columns, never rename or remove existing primary key or foreign key columns" to ensure safe auto-migration.

### 6.1 Three-Layer Manifest Validation (`manifest/validate.go`)

Every manifest passes through three validation gates before the Differ runs:

1. **Structural validation** — JSON parses correctly, all required fields present, types match (e.g., `method` is one of GET/POST/PUT/PATCH/DELETE). Fast, catches LLM output corruption.

2. **Referential integrity** — Every `route.script` references a name that exists in `scripts[]`. Every table referenced in Tengo `db.*` calls exists in `schemas[]`. Every `seeds[].table` exists in `schemas[]`. Every `references` foreign key points to a valid `table.column`.

3. **Script compilation** — Each script in `scripts[]` is passed through Tengo's `Compile()`. Catches syntax errors before the script ever reaches the runtime. This is cheaper than the full in-memory dry run (`ScriptValidationStarted`) because it only checks syntax, not execution.

If any gate fails, the engine emits `ManifestValidationFailed` (new event) with the specific error, and the old manifest remains active. The TUI shows the error and invites the user to rephrase.

## 7. Vibe Standard Library (Tengo Sandbox Contract)

All Tengo scripts execute in a hermetically sealed sandbox. They can only interact with the system through these functions:

### db (DataStore)
- `db.query(sql, params) -> []Row` — parameterized SELECT
- `db.query_one(sql, params) -> Row|undefined` — single row SELECT
- `db.insert(table, data) -> Row` — INSERT, returns row with auto ID
- `db.update(table, id, data) -> Row` — UPDATE by primary key
- `db.delete(table, id) -> bool` — DELETE by primary key
- `db.count(table, where) -> int` — COUNT with optional filter

### request (HTTP Context)
- `request.param(name) -> string` — URL path parameter
- `request.query(name) -> string` — query string parameter
- `request.body() -> object` — parsed JSON body
- `request.header(name) -> string` — request header
- `request.method() -> string` — HTTP method
- `request.auth() -> object|undefined` — parses the `Authorization` header. For `Bearer <token>`, attempts Base64-decode of the token payload as JSON (e.g., `{"role": "admin", "user_id": 1}`). If Base64 decode fails (token is opaque), returns `{"raw_token": "<token>"}` instead of `undefined`, enabling simple token-matching auth (`if request.auth().raw_token == "secret"`). Returns `undefined` only when no Authorization header is present.

### response (HTTP Response)
- `response.json(data, status?) -> void` — JSON response (default 200)
- `response.error(status, msg) -> void` — error response
- `response.header(name, value) -> void` — set response header
- `response.redirect(url) -> void` — 302 redirect

### date (Date Utilities)
- `date.now() -> string` — ISO 8601 timestamp
- `date.diff_days(a, b) -> int` — days between two dates
- `date.add_days(d, n) -> string` — add N days
- `date.format(d, fmt) -> string` — format date string

### crypto (Tokens & IDs)
- `crypto.hash(str) -> string` — SHA-256 hash
- `crypto.uuid() -> string` — UUIDv4
- `crypto.random(min, max) -> int` — random integer in range

### log (Observability)
- `log.info(msg) -> void` — emits LogEmitted event (info level)
- `log.warn(msg) -> void` — emits LogEmitted event (warn level)
- `log.error(msg) -> void` — emits LogEmitted event (error level)

**Intentionally excluded:** filesystem access, network calls (`http.get`), subprocess execution (`exec`), external module imports. The sandbox is sealed.

**Security model:** All `db.*` functions use parameterized queries only. No raw SQL execution. This prevents SQL injection in AI-generated code.

### 7.1 SQLite Type Mapping

SQLite has a flexible type affinity system. The manifest's column types map to SQLite storage as follows:

| Manifest Type | SQLite Affinity | Storage Format | stdlib Handling |
|---|---|---|---|
| `INTEGER` | INTEGER | Native int64 | Direct pass-through |
| `TEXT` | TEXT | UTF-8 string | Direct pass-through |
| `REAL` | REAL | 64-bit float | Direct pass-through |
| `BOOLEAN` | INTEGER | 0 or 1 | stdlib converts to/from `true`/`false` in Tengo |
| `DATE` | TEXT | `YYYY-MM-DD` | `date.*` functions accept and return this format |
| `DATETIME` | TEXT | ISO 8601 (`YYYY-MM-DDTHH:MM:SSZ`) | `date.*` functions handle conversion |

The `store` module handles this mapping transparently. Tengo scripts always work with natural types (booleans, date strings) — never raw SQLite integers for booleans.

### 7.2 CORS (Cross-Origin Resource Sharing)

Frontend engineers running `localhost:3000` (React/Vue) against `localhost:8080` (VibeServe) will hit CORS errors immediately.

**Default behavior**: In both `dev` and `up` modes, VibeServe responds to all requests with permissive CORS headers:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```

**Configuration**: `cors: true` (default) in `.vibe/config.yaml`. Set to `false` to disable.

Preflight `OPTIONS` requests are handled automatically by the router before reaching Tengo scripts.

## 8. TUI Design (Bubble Tea)

### 8.1 Layout

Split-pane terminal UI:

- **Left pane (60%)**: Conversation — scrollable message history + input field
- **Right pane (40%)**: Live Dashboard — four collapsible panels:
  - Routes panel: live route table
  - DB State panel: table names, row counts, data preview
  - HTTP Trace panel: recent requests with status + timing
  - Snapshots panel: undo history, selectable for restore

### 8.2 Component Tree

```
RootModel
├── HeaderBar         (project name, server URL, help shortcut)
├── SplitPane         (horizontal, resizable via keybind)
│   ├── ConversationView
│   │   ├── MessageList   (scrollable, supports streaming LLM output)
│   │   └── InputField    (Enter to submit, Shift+Enter for newline)
│   └── DashboardView
│       ├── RoutesPanel
│       ├── DBStatePanel
│       ├── HTTPTracePanel
│       └── SnapshotPanel
└── StatusBar         (keybind hints, connection status)
```

### 8.3 Key Interactions

- `Tab`: switch focus between left and right panes
- `j/k` or `Up/Down` (right pane): cycle between dashboard panels
- `Enter` (right pane): expand/collapse panel
- `t` (on route): auto-generate test request from manifest's `request_body` schema (random realistic data for POST/PUT), execute, show result in HTTP Trace
- `u` (on snapshot): restore snapshot with confirmation
- `Ctrl+C`: quit

### 8.4 Visual Feedback

- **Flash-highlight**: new/updated items flash green for 1 second
- **Removal flash**: removed items flash red before disappearing
- **Glow effect**: HTTP trace entries glow briefly on new request
- **Streaming text**: LLM responses stream character-by-character in left pane
- **Cascade update**: schema -> route -> script updates appear sequentially (~500ms)
- **Graceful collapse**: at 80x24 terminals, right pane collapses to Routes + Trace only

## 9. Web Console

Served at `localhost:<port>/_console`. Embedded via Go's `embed` package. Communicates with engine via WebSocket (same events as TUI).

**Purpose**: Depth inspection that TUI can't provide. Not a duplicate of TUI.

### Tabs

1. **API Explorer** — Postman-like request builder. Auto-generates request bodies from manifest schema. Send button executes real requests.

2. **Database Browser** — Full table browsing with filtering. Column schema inspection. Row counts and DB size. Includes a **[Download SQLite File]** button for exporting the raw `.vibe/state.db`.

3. **Script Viewer** — Read-only Tengo source viewer. Shows stdlib function usage sidebar. Displays validation status and last execution metrics.

4. **Trace Timeline** — Step-by-step execution trace for each request. Shows every stdlib call, its timing, and return value. The "magnifying glass" for debugging.

5. **Manifest** — Raw JSON view of current System Manifest.

**Tech stack**: Single-page app built with Preact + HTM (no build step, no JSX transpilation). No external CDN dependencies — all assets embedded in the binary via Go's `embed` package. Static assets in `internal/web/static/` must be minified/compressed before embedding to control binary size.

## 10. CLI Subcommands

```
vibeserve dev      # Mutable session: TUI + HTTP server + web console
vibeserve up       # Read-only server: runs from existing .vibe/ state
vibeserve export   # Generate OpenAPI 3.x + Go server from .vibe/
vibeserve init     # Create .vibe/ directory + config
vibeserve undo     # Restore previous snapshot (non-interactive shortcut)
vibeserve routes   # Print current route table
vibeserve config   # Manage LLM provider settings
vibeserve version  # Print version
```

### Mode Comparison

| Capability | `dev` | `up` | `export` |
|---|---|---|---|
| TUI | Full split-pane | None (stdout logs) | None (one-shot) |
| HTTP Server | Yes | Yes | No |
| Web Console | Yes (`/_console`) | Yes (read-only) | No |
| LLM Required | Yes | No | No |
| Manifest | Mutable (AI updates) | Frozen | Read-only input |
| Schema Changes | Yes (conversation) | No | N/A |
| DB Writes | Via API + migration | Via API only | N/A |

### `vibeserve dev` Startup Sequence

1. Check `.vibe/` exists; if not, run implicit init
2. Load config, resolve LLM provider
3. Start Engine, wire modules via interface injection
4. Start HTTP server on `:8080` (or `--port`)
5. Start web console on `/_console`
6. Launch Bubble Tea TUI
7. If existing `manifest.json` found, hydrate routes + DB
8. Show welcome message

### `vibeserve export` Output

```
exported/
├── main.go              # Chi router setup
├── handlers/            # One file per resource
├── models/              # Go structs from schema
├── db/
│   ├── migrations/      # SQL migration files
│   └── db.go            # SQLite connection
├── openapi.yaml         # Full OpenAPI 3.x spec
├── go.mod
├── go.sum
├── Dockerfile           # Ready-to-deploy
└── README.md            # API reference
```

Export uses Go `text/template` to render code from templates. Template quality can be improved independently of core logic.

`vibeserve export --target openapi` produces only `openapi.yaml`.

## 11. Configuration

```yaml
# .vibe/config.yaml
provider: claude
api_key_env: ANTHROPIC_API_KEY
model: claude-sonnet-4-6-20250514
ollama_host: http://localhost:11434
server:
  port: 8080
  host: localhost
  cors: true
```

API keys are never stored in config — only the environment variable name. The binary reads the key from the environment at runtime.

## 12. Runtime State

```
.vibe/
├── config.yaml          # LLM provider + server settings
├── state.db             # SQLite database
├── manifest.json        # Current System Manifest
└── snapshots/
    ├── 001_initial.db
    ├── 002_add_loyalty.db
    └── ...
```

The `.vibe/` directory should be gitignored. It contains ephemeral development state.

Snapshots are full SQLite file copies, created automatically before every schema change. Naming includes a sequential ID and a human-readable suffix derived from the change description.

## 13. MVP Phasing

### Phase 1: The Skeleton (vibeserve up)
Get a static manifest-driven server running. No LLM, no TUI. User hand-writes a `manifest.json`, and VibeServe hydrates routes + DB + scripts from it.

Validates: manifest schema, trie router, SQLite store, Tengo runtime, Vibe stdlib, HTTP serving.

**Hard gate**: Parameterized queries must be enforced from day one. The `db.*` stdlib functions must reject any attempt to concatenate user input into SQL strings. If this isn't locked down in Phase 1, it becomes a painful retrofit later.

### Phase 2: The Brain (LLM integration)
Add the pluggable LLM layer. `vibeserve dev` in CLI-REPL mode (no TUI yet). User types, LLM produces manifest, Differ applies changes.

Validates: LLM provider abstraction, manifest diffing, auto-migration, snapshot/undo.

### Phase 3: The Face (Bubble Tea TUI)
Replace the simple REPL with the full split-pane TUI. Flash updates, streaming, cascade feedback.

Validates: Bubble Tea event integration, concurrent TUI + HTTP server, visual feedback loop.

**Demo-ready feature**: The `t` key quick-test should auto-generate realistic random data from the manifest's `request_body` schema (random names, dates, IDs) for POST/PUT endpoints. This makes demo videos visually compelling.

### Phase 4: The Magnifying Glass (Web Console)
Add the embedded web console with all five tabs.

Validates: WebSocket event streaming, API explorer, trace timeline, embedded static assets.

### Phase 5: The Exit (Export)
Implement OpenAPI + Go server export.

Validates: Template rendering, idiomatic Go output, zero-mod deployment.

## 14. Key Go Dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `modernc.org/sqlite` | Pure-Go SQLite (no CGO) |
| `github.com/d5/tengo/v2` | Tengo scripting VM |
| `gopkg.in/yaml.v3` | Config parsing |
| `nhooyr.io/websocket` | WebSocket for web console |

All pure Go — no CGO required. Single `go build` produces the final binary.
