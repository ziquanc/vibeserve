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

VibeServe is a single-binary CLI that turns natural language into a running API server. Describe what you want, and VibeServe creates the database, routes, business logic, and seed data — all live, all instantly testable. When you're ready, export to a production Go or Express.js project.

```
 Describe                    Prototype                    Ship
 ───────                    ─────────                    ────
 "Create a task API    ──>   API running at          ──>  vibeserve export
  with users and             localhost:8080                --format express | next
  projects"                  ER diagram at /_blueprint     --db postgres
                             Swagger at /_swagger          --typescript
```

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/ziquanc/vibeserve/main/install.sh | sh

# Option A: Start from a template (instant, no AI needed)
vibeserve init --template ecommerce
vibeserve up

# Option B: Describe what you want (AI-powered)
vibeserve
vibe> Create a task management API with users, projects, and tasks.
```

That's it. Your API is running. Test it:

```bash
curl http://localhost:8080/users
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"Ship v1","priority":"high"}'
```

### Starter Templates

```bash
vibeserve init --template blog        # Posts, comments, categories, tags
vibeserve init --template ecommerce   # Products, orders, cart, coupons, reviews
vibeserve init --template saas        # Teams, members, plans, subscriptions, billing
```

Each template includes auth, state machines, admin routes, and seed data. Modify with AI afterward: `vibeserve` → "add a newsletter feature".

## Three Ways to Build

### 1. Conversational Mode (default)

Describe what you want. VibeServe proposes a blueprint with an ER diagram, you review and approve or refine with natural language, and it builds.

```
vibe> Create a task management API with users, projects, and tasks.

  Blueprint — 4 steps

  1. Create tables: users (id, name, email, role), projects (id, name, owner_id FK),
     tasks (id, project_id FK, assignee_id FK, title, priority, due_date, status)
  2. Add CRUD routes + POST /tasks/:id/assign
  3. Add analytics: GET /projects/:id/stats, GET /users/:id/workload
  4. Seed sample data

  ER Diagram:  http://localhost:8080/_blueprint

  [y] approve  |  type feedback to refine  |  [n] cancel

vibe> y

  ✓ Applied: 3 schema changes, 15 routes, 15 scripts

  Seed sample data? 3 tables, 12 rows [y/N]

vibe> y
  ✓ Seeding users (3 rows)
  ✓ Seeding projects (2 rows)
  ✓ Seeding tasks (7 rows)
```

Type natural language to refine: "add a comments feature" or "rename status to state". Type `y` to approve.

### 2. Proxy Mode — API builds itself from HTTP requests

```bash
vibeserve --proxy
```

Just send requests. VibeServe creates whatever doesn't exist yet:

```bash
# No /pets table exists. This creates it + inserts the row:
curl -X POST http://localhost:8080/pets \
  -d '{"name":"Peddy","type":"dog","age":3}'
# => {"id":1,"name":"Peddy","type":"dog","age":3}

# Triggers relationship detection — creates visits table with pet_id FK:
curl http://localhost:8080/pets/1/visits

# FK auto-detection — creates owners table with pet_id → pets.id:
curl -X POST http://localhost:8080/owners \
  -d '{"name":"Kent","pet_id":1}'
```

The proxy uses the LLM to design proper schemas with correct types, constraints, and foreign keys — not just guessing from body fields.

Smart features: FK detection, relationship routes, soft delete, entity vs action detection, rate limiting (30 gen/min).

### 3. MCP Server — let AI coding assistants drive

```bash
vibeserve mcp
```

AI tools like Claude Code and Cursor can create and manage your API programmatically via the [Model Context Protocol](https://modelcontextprotocol.io/).

**Setup:**

```json
{
  "mcpServers": {
    "vibeserve": { "command": "vibeserve", "args": ["mcp"] }
  }
}
```

**9 tools available:**

| Tool | What it does |
|------|-------------|
| `create_api` | Create API from natural language |
| `add_feature` | Add tables, columns, or routes |
| `list_routes` | Show all endpoints |
| `list_tables` | Show tables with columns and row counts |
| `query_data` | Read-only SQL queries |
| `insert_data` | Insert rows |
| `export_project` | Export to Go or Express.js |
| `undo` | Roll back last change |
| `get_api_status` | API summary |

**Example:** You tell Claude Code "build a React app for managing tasks" — it calls `create_api` to generate the backend, calls `list_routes` to discover endpoints, then generates React components that fetch from the live API. Backend and frontend built together.

## Export to Production

Graduate from prototype to production-ready code:

```
vibeserve export [flags] [output-dir]

Flags:
  --format    go | express | next | fullstack  (default: go)
  --db        sqlite | postgres                (default: sqlite)
  --typescript                                 Generate TypeScript interfaces
```

### Go

```bash
vibeserve export ./my-api
cd my-api && go run ./cmd/api
```

Chi router, sqlx, typed models, Dockerfile, OpenAPI spec.

### Express.js

```bash
vibeserve export --format express ./my-api
cd my-api && npm install && npm start
```

Full security stack: helmet, CORS, JWT auth, rate limiting, bcrypt, express-validator, compression.

### Express.js + PostgreSQL

```bash
vibeserve export --format express --db postgres ./my-api
psql -d mydb -f my-api/schema.sql    # Create tables with FK indexes
psql -d mydb -f my-api/seed.sql      # Seed data (optional)
cd my-api && npm install && npm start
```

Generates `schema.sql` + `seed.sql`. Uses `pg` driver with async/await, `$1/$2` parameterized queries, `RETURNING *`.

### Express.js + TypeScript

```bash
vibeserve export --format express --typescript ./my-api
```

Adds `src/types.ts` with typed interfaces and `tsconfig.json`:

```typescript
export interface User {
  id: number;
  name: string;
  email: string;
  created_at: string;
  deleted_at: string | null;
}

export interface CreateUserInput {
  name: string;    // required
  email: string;   // required
}

export interface UpdateUserInput {
  name?: string;   // optional
  email?: string;
}
```

### Next.js Admin Panel

```bash
vibeserve export --format next ./my-frontend
cd my-frontend && npm install && npm run dev
```

Generates a full Next.js 15 admin panel with shadcn/ui:

- Dashboard with resource cards
- Per-table CRUD pages: data table, create form, detail view, edit form
- Typed API client (`lib/api.ts`) with all CRUD functions
- TypeScript interfaces (`lib/types.ts`)
- Sidebar navigation with lucide-react icons
- Search, sort, pagination on all list pages
- Tailwind CSS v4 + shadcn/ui components

The frontend proxies `/api/*` to your VibeServe server — works in dev, configurable for production via `API_URL`.

### Full-Stack (Express + Next.js)

```bash
vibeserve export --format fullstack ./my-app
cd my-app && npm run install:all && npm run dev
```

Generates a monorepo with both backend and frontend:

```
my-app/
  backend/     Express.js API
  frontend/    Next.js admin panel
  package.json npm workspaces + concurrently
```

One command starts both: `npm run dev`. Combine with `--db postgres --typescript` for production.

### What's included in every export

| Feature | Description |
|---------|-------------|
| Auth | Auto-generated register, login, JWT + refresh tokens, /me, password reset |
| State machines | Lifecycle workflows with role + condition guards (orders: draft→shipped→delivered) |
| Soft delete | `created_at`, `updated_at`, `deleted_at` on every table |
| Pagination | `LIMIT`/`OFFSET` with configurable defaults |
| Search & filtering | `?sort=name&order=desc&search=term&status=active` |
| FK auto-inference | `user_id` columns auto-linked to `users` table on export |
| Input validation | Type-aware validation on all routes |
| Error handling | try/catch with Express error middleware |
| Security | helmet, CORS, JWT, rate limiting, bcrypt |
| Docker | Production Dockerfile with health check |
| OpenAPI | Auto-generated OpenAPI 3.0.3 spec |

## Built-in Tools

### Web Dashboard

Once the server is running:

| URL | What |
|-----|------|
| `localhost:8080/_console` | Browse tables, routes, scripts, HTTP traces |
| `localhost:8080/_blueprint` | ER diagram with fullscreen view and interactive highlighting |
| `localhost:8080/_swagger` | Swagger UI — live API docs |

### API Testing

```bash
vibeserve test              # Run CRUD lifecycle tests for every table
vibeserve test --port 3000  # Test against a different port
```

```
  ✓ users: POST /users — created id=1
  ✓ users: GET /users — returns array
  ✓ users: GET /users/:id — got id=1
  ✓ users: PUT /users/:id — updated
  ✓ users: DELETE /users/:id — soft deleted
  ✓ users: GET /users/:id (after delete) — correctly returns 404
  6 passed, 0 failed
```

### Watch Mode

Keep your exported project in sync as you iterate:

```bash
vibeserve watch --export ./my-api --format express --db postgres
```

Watches `.vibe/manifest.json`. When it changes (via chat, MCP, or direct edit), auto re-exports the project and generates PostgreSQL migration SQL in `migrations/`.

### Other Commands

| Command | Description |
|---------|-------------|
| `vibeserve init --template <name>` | Start from a template (blog, ecommerce, saas) |
| `vibeserve watch --export <dir>` | Server + auto re-export on manifest changes |
| `vibeserve diff` | Show current API — tables, columns, routes |
| `vibeserve routes` | Print route table |
| `vibeserve undo` | Restore last database snapshot |
| `vibeserve up` | Start server without AI (existing manifest) |

## Install

**Quick install (macOS & Linux):**
```bash
curl -fsSL https://raw.githubusercontent.com/ziquanc/vibeserve/main/install.sh | sh
```

**Go install:**
```bash
go install github.com/ziquanc/vibeserve/cmd/vibeserve@latest
```

**Manual:** Download from [GitHub Releases](https://github.com/ziquanc/vibeserve/releases).

## LLM Providers

| Provider | Setup |
|----------|-------|
| **Claude** (Anthropic) | API key from [console.anthropic.com](https://console.anthropic.com) |
| **OpenAI-compatible** | z.ai, x.ai, Groq, Together, OpenRouter — any OpenAI-compatible endpoint |
| **Ollama** | Local models, no API key needed — [ollama.com](https://ollama.com) |

The setup wizard runs on first launch and saves config to `.vibe/config.yaml`.

## Privacy

VibeServe runs entirely on your machine. No server, no account, no telemetry.

- Your code stays local — manifests, database, scripts all in `.vibe/`
- LLM calls go directly to your provider — VibeServe never proxies or stores them
- Use Ollama for fully offline mode — nothing leaves your machine

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
