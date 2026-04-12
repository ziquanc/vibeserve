<p align="center">
  <img src="assets/banner.png" width="100%" alt="VibeServe" />
</p>

<h1 align="center">VibeServe</h1>

<p align="center">
  <strong>Describe your API. Get a running server.</strong>
</p>

<p align="center">
  <a href="https://github.com/ziquanc/vibeserve/releases"><img src="https://img.shields.io/github/v/release/ziquanc/vibeserve?color=7C3AED&label=release" alt="Release"></a>
  <a href="https://github.com/ziquanc/vibeserve/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-22C55E" alt="MIT License"></a>
  <a href="#install"><img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-06B6D4" alt="Platform"></a>
  <a href="https://github.com/ziquanc/vibeserve/issues"><img src="https://img.shields.io/github/issues/ziquanc/vibeserve?color=F59E0B" alt="Issues"></a>
</p>

---

VibeServe is an AI-powered CLI that turns natural language into a running API server. Describe what you want in plain English (or any language), and VibeServe uses an LLM to create the database schema, API routes, business logic, and seed data — all live, all instantly testable.

**What used to take hours now takes seconds.** No boilerplate, no scaffolding, no framework lock-in.

**How it speeds up your workflow:**

| Traditional | With VibeServe |
|-------------|----------------|
| Design schema, write migrations, build routes, write handlers, add validation, create seed data | Type one sentence. Done. |
| Switch between database tool, editor, and API tester | Everything in one terminal — schema, routes, and live API running together |
| Prototype in one stack, rewrite for production | Prototype instantly, export to Go or Express.js when ready to ship |
| Manually add each new endpoint | Proxy mode auto-generates endpoints from your HTTP requests |

**Who is it for:**
- **Frontend developers** who need a real API right now to build against — not a mock server, an actual working backend
- **Indie hackers** prototyping a new product and need to ship an MVP fast
- **Teams** spinning up internal tools, admin panels, or microservices without boilerplate
- **Learners** exploring API design — see your ideas come to life in seconds

> **New in v0.2** — MCP server for AI coding assistants, Express.js export with PostgreSQL + TypeScript, soft delete, search/filtering, and proxy mode that auto-builds your API from HTTP requests.

## Features

| Feature | Description |
|---------|-------------|
| **Conversational API Design** | Describe your API in natural language. VibeServe creates tables, routes, scripts, and seed data. |
| **Blueprint Review** | AI proposes an architecture with heuristic scoring before writing code. Approve, refine, or reject. |
| **Live Server** | Your API is running at `localhost:8080` the moment it's created. Test with curl immediately. |
| **Architectural Heuristics** | Scores blueprints 0-10 for depth. Detects state machines, computed endpoints, validation guards. |
| **Web Console** | Built-in dashboard at `/_console` — browse tables, routes, scripts, and live HTTP traces. |
| **ER Diagram** | Auto-generated entity relationship diagram at `/_blueprint` with interactive table highlighting. |
| **Swagger UI** | Live API documentation at `/_swagger` — generated from your manifest in real-time. |
| **Undo** | Every schema change creates a snapshot. Type `/undo` to roll back instantly. |
| **Export** | `vibeserve export` generates a standalone Go project — Chi router, sqlx, typed models, Dockerfile. Also supports Express.js with `--format express` and PostgreSQL with `--db postgres`. |
| **TypeScript** | `--typescript` flag generates type interfaces (User, CreateUserInput, UpdateUserInput) from your schemas. |
| **Soft Delete** | All tables auto-include `created_at`, `updated_at`, `deleted_at`. Delete sets `deleted_at` instead of removing rows. |
| **Search & Filtering** | Generated list endpoints support `?sort=name&order=desc&search=term&status=active` out of the box. |
| **MCP Server** | `vibeserve mcp` — lets AI coding assistants (Claude Code, Cursor) create and manage APIs programmatically. |
| **API Tests** | `vibeserve test` — auto-generated CRUD lifecycle tests for every table. |
| **Proxy Mode** | `vibeserve --proxy` — auto-generates endpoints from unmatched HTTP requests. Detects foreign keys and builds relationships. |
| **Pluggable LLM** | Works with Claude, OpenAI-compatible APIs (z.ai, x.ai, Groq), and local Ollama models. |
| **Single Binary** | One file. No runtime dependencies. No Docker required. Just download and run. |

## Install

### Quick Install (macOS & Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/ziquanc/vibeserve/main/install.sh | sh
```

This downloads the latest release binary for your platform and puts it in `/usr/local/bin`.

### Go Install

If you have Go installed:

```bash
go install github.com/ziquanc/vibeserve/cmd/vibeserve@latest
```

### Manual Download

Download the binary for your platform from [GitHub Releases](https://github.com/ziquanc/vibeserve/releases) and add it to your PATH.

## Quick Start

```bash
# Start VibeServe (first run launches setup wizard)
vibeserve

# Choose your AI provider, enter API key, done.
# Now type a prompt:
```

```
vibe> Create a task management API with users, projects, and tasks.
      Tasks have priorities and due dates. Users can be assigned to tasks.

VibeServe: Blueprint plan: 4 steps

  1. Create users table with id, name, email, role
  2. Create projects table with id, name, owner_id
  3. Create tasks table with id, project_id, assignee_id, title, priority, due_date, status
  4. Add routes: CRUD for all, plus POST /tasks/:id/assign and PATCH /tasks/:id/complete

  [y] approve  |  type feedback to refine  |  [n] cancel

blueprint> y

VibeServe: Done! Schema: +3 tables, Routes: +12, Scripts: +12
```

```bash
# Test it immediately
curl http://localhost:8080/users
curl -X POST http://localhost:8080/tasks -d '{"title":"Ship v1","priority":"high"}'
```

## Commands

| Command | Description |
|---------|-------------|
| `vibeserve` | Start in interactive mode (default) |
| `vibeserve --proxy` | Start with proxy/auto-evolve mode — unmatched requests auto-generate endpoints |
| `vibeserve up` | Start server from existing manifest (no AI) |
| `vibeserve export [dir]` | Export standalone Go project |
| `vibeserve export --format express [dir]` | Export standalone Express.js project with security middleware |
| `vibeserve export --format express --db postgres [dir]` | Export Express.js project with PostgreSQL (generates schema.sql + seed.sql) |
| `vibeserve export --format express --typescript [dir]` | Export Express.js with TypeScript type definitions |
| `vibeserve test` | Run auto-generated CRUD tests against the running server |
| `vibeserve diff` | Show current API summary — tables, columns, routes |
| `vibeserve mcp` | Start MCP server for AI coding assistants (stdio transport) |
| `vibeserve routes` | Print route table |
| `vibeserve undo` | Restore last database snapshot |
| `vibeserve version` | Print version |

### In-App Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/routes` | List all API routes |
| `/status` | Show project status |
| `/undo` | Rollback last change |
| `/quit` | Exit VibeServe |

## Web Tools

Once running, these are available in your browser:

| URL | What |
|-----|------|
| `http://localhost:8080/_console` | Web console — tables, routes, scripts, HTTP traces |
| `http://localhost:8080/_blueprint` | Blueprint preview — ER diagram, schemas, routes |
| `http://localhost:8080/_swagger` | Swagger UI — live API documentation |

## Export

Graduate from prototype to production. Two formats:

### Go (default)

```bash
vibeserve export ./my-api
```

Generates a complete Go project:

```
my-api/
  cmd/api/main.go          # Chi router + middlewares + graceful shutdown
  internal/model/           # Typed Go structs from your schemas
  internal/repository/      # Store interface + SQLite implementation
  internal/handler/         # HTTP handlers translated from scripts
  go.mod                    # Ready to build
  Dockerfile                # Multi-stage build
  openapi.yaml              # OpenAPI 3.0.3 spec
  README.md                 # Auto-generated docs
  state.db                  # Your data, copied over
```

```bash
cd my-api && go run ./cmd/api
```

### Express.js

```bash
vibeserve export --format express ./my-api
```

Generates a production-ready Express.js project with full security:

```
my-api/
  server.js                 # Express + helmet + CORS + rate-limit + JWT + compression
  package.json              # All dependencies pre-configured
  .env.example              # Environment variables template
  .gitignore                # node_modules, .env, *.db excluded
  Dockerfile                # Production build with health check
  openapi.yaml              # OpenAPI 3.0.3 spec
  README.md                 # Auto-generated docs with route table
  src/
    middleware/auth.js       # JWT authentication + bcrypt password hashing
    middleware/validate.js   # express-validator wrapper
    models/database.js       # better-sqlite3 with prepared statements
    routes/
      index.js              # Route aggregator
      {resource}.js         # Per-resource route handlers with input validation
    data/state.db            # Your data, copied over
```

#### PostgreSQL mode

```bash
vibeserve export --format express --db postgres ./my-api
```

Swaps SQLite for PostgreSQL. The generated project uses the `pg` (node-postgres) driver with async/await:

```
my-api/
  schema.sql                # CREATE TABLE statements — run against your PostgreSQL database
  seed.sql                  # INSERT statements with seed data (wrapped in transaction)
  server.js                 # Same Express security stack
  src/
    models/database.js      # pg Pool with async CRUD helpers, $1/$2 parameterized queries
    routes/{resource}.js    # Async handlers with await, RETURNING * for inserts/updates
    ...
```

```bash
# 1. Set up your database
psql -d your_database -f schema.sql
psql -d your_database -f seed.sql    # optional

# 2. Configure and run
cp .env.example .env                 # Set DATABASE_URL=postgresql://...
npm install && npm start
```

You can also set the database in your manifest (`"database": "postgres"`) so exports default to PostgreSQL without the `--db` flag.

**Security stack included:**
- **helmet** — security HTTP headers
- **cors** — configurable cross-origin (defaults to localhost)
- **express-rate-limit** — 100 req/15min general, 5 req/15min auth routes
- **jsonwebtoken + bcryptjs** — JWT auth with password hashing (JWT_SECRET required on startup)
- **express-validator** — type-aware input validation on all routes
- **morgan** — request logging
- **compression** — gzip responses
- **1MB body size limit**
- **Error handling** — try/catch on all routes with Express error middleware
- **Pagination** — list endpoints support limit/offset (default 50 per page)
- **Search & filtering** — `?sort=name&order=desc&search=term&field=value`
- **Soft delete** — all tables include `created_at`, `updated_at`, `deleted_at`
- **Foreign key indexes** — PostgreSQL export auto-creates indexes on FK columns and `deleted_at`

```bash
cd my-api && npm install && npm start
```

#### TypeScript support

```bash
vibeserve export --format express --typescript ./my-api
```

Generates `src/types.ts` with typed interfaces and a `tsconfig.json`:

```typescript
export interface User {
  id: number;
  name: string;
  email: string;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
}

export interface CreateUserInput {
  name: string;
  email: string;
}

export interface UpdateUserInput {
  name?: string;
  email?: string;
}

export interface ListOptions {
  limit?: number;
  offset?: number;
  sort?: string;
  order?: 'asc' | 'desc';
  search?: string;
}
```

Also adds `typescript` and all `@types/*` packages to `package.json`.

## MCP Server

`vibeserve mcp` starts a [Model Context Protocol](https://modelcontextprotocol.io/) server so AI coding assistants can manage your API programmatically.

**Setup (Claude Code / Cursor):**

```json
{
  "mcpServers": {
    "vibeserve": {
      "command": "vibeserve",
      "args": ["mcp"]
    }
  }
}
```

**Available tools:**

| Tool | Description |
|------|-------------|
| `create_api` | Create an API from natural language description |
| `add_feature` | Add tables, columns, or routes to an existing API |
| `list_routes` | Show all API endpoints |
| `list_tables` | Show all tables with columns and row counts |
| `query_data` | Run read-only SQL queries |
| `insert_data` | Insert rows into tables |
| `export_project` | Export to Go or Express.js |
| `get_api_status` | Show API summary |
| `undo` | Roll back last change |

**Example workflow in Claude Code:**

```
You: "Create a blog API with users, posts, and comments"
Claude Code → calls create_api → API is live at localhost:8080

You: "Now build a React frontend that fetches from this API"
Claude Code → calls list_routes to see endpoints
Claude Code → generates React components with correct fetch calls
```

## Testing

```bash
# Start the server first
vibeserve up &

# Run auto-generated tests
vibeserve test
```

VibeServe reads your manifest and runs a CRUD lifecycle test for each table:

```
  ✓ users: POST /users — created id=1
  ✓ users: GET /users — returns array
  ✓ users: GET /users/:id — got id=1
  ✓ users: PUT /users/:id — updated
  ✓ users: DELETE /users/:id — soft deleted
  ✓ users: GET /users/:id (after delete) — correctly returns 404 after soft delete

6 passed, 0 failed
```

Use `--port` to test against a different port: `vibeserve test --port 3000`

## Proxy Mode (Auto-Evolve)

Enable with `vibeserve --proxy`. Instead of returning 404 for unmatched routes, VibeServe auto-generates the endpoint:

```bash
vibeserve --proxy
```

```bash
# No /products route exists yet
curl -X POST http://localhost:8080/products -d '{"name":"Widget","price":9.99}'

# VibeServe detects the unmatched route, uses LLM to generate:
#   - products table (name TEXT, price REAL)
#   - POST /products route
#   - Registers the route permanently
# Returns: {"id":1,"name":"Widget","price":9.99}
# Header: X-VibeServe-Generated: true

# Next request is instant — route already exists
curl http://localhost:8080/products
# [{"id":1,"name":"Widget","price":9.99}]

# Another new endpoint — API keeps growing
curl http://localhost:8080/products/1/reviews
# Auto-generates reviews table linked to products
```

Each new request potentially adds new tables and routes. Your API builds itself as you use it. All generated routes persist in the manifest and survive restarts.

**Smart features:**
- **Foreign key detection** — `POST /order {"product_id": 1}` automatically creates FK reference to the `products` table
- **Relationship routes** — `GET /users/:id/posts` auto-creates `posts` table with `user_id` FK if it doesn't exist
- **Soft delete** — all generated DELETE endpoints use soft delete
- **Rate limiting** — max 30 route generations per minute to prevent API credit burn

## LLM Providers

VibeServe supports any LLM that can output JSON:

| Provider | Setup |
|----------|-------|
| **Claude** (Anthropic) | API key from [console.anthropic.com](https://console.anthropic.com) |
| **OpenAI-compatible** | z.ai, x.ai, Groq, Together, OpenRouter — any OpenAI-compatible endpoint |
| **Ollama** | Local models, no API key needed — [ollama.com](https://ollama.com) |

The setup wizard runs on first launch and saves config to `.vibe/config.yaml`.

## How It Works

```
You describe an API in plain language
        |
   AI analyzes your request and creates a plan
        |
   Blueprint proposed -----> You review (approve/refine/cancel)
        |
   Manifest generated -----> JSON schema + routes + Tengo scripts
        |
   Auto-migrated ---------> SQLite tables created/updated
        |
   Routes registered ------> HTTP server serves your API
        |
   Test immediately -------> curl localhost:8080/your-routes
```

Or skip the conversation entirely with **proxy mode**:

```
You send an HTTP request to a route that doesn't exist
        |
   AI analyzes the request (method, path, body)
        |
   Auto-generates table + route + handler
        |
   Route registered permanently
        |
   Response returned to your original request
```

VibeServe stores everything in a `.vibe/` directory:
- `manifest.json` — the API definition (schemas, routes, scripts, seeds)
- `config.yaml` — LLM provider settings
- `state.db` — SQLite database with your data
- `snapshots/` — database snapshots for undo

## Privacy

VibeServe runs entirely on your machine. There is no server, no account, no telemetry.

- **Your code stays local.** Manifests, database, scripts — all in `.vibe/` on your disk.
- **No data collection.** VibeServe does not phone home, track usage, or send analytics.
- **LLM calls go directly to your provider.** Your prompts are sent to the API you configured (Claude, OpenAI-compatible, or local Ollama) — VibeServe never proxies or stores them.
- **Offline mode.** Use Ollama with a local model and nothing leaves your machine at all.

## Contributing

```bash
git clone https://github.com/ziquanc/vibeserve.git
cd vibeserve
go build -o vibeserve ./cmd/vibeserve
./vibeserve
```

Requires Go 1.23+. No CGO — builds anywhere Go runs.

## Community

- [Issues](https://github.com/ziquanc/vibeserve/issues) — Bug reports and feature requests
- [Discussions](https://github.com/ziquanc/vibeserve/discussions) — Questions and ideas

## License

[MIT](LICENSE) — use it however you want.
