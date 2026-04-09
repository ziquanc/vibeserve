<p align="center">
  <img src="assets/banner.png" width="100%" alt="VibeServe" />
</p>

<h1 align="center">VibeServe</h1>

<p align="center">
  <strong>Describe your API. Get a running server.</strong>
</p>

<p align="center">
  <a href="https://github.com/vibeserve/vibeserve/releases"><img src="https://img.shields.io/github/v/release/vibeserve/vibeserve?color=7C3AED&label=release" alt="Release"></a>
  <a href="https://github.com/vibeserve/vibeserve/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-22C55E" alt="MIT License"></a>
  <a href="#install"><img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-06B6D4" alt="Platform"></a>
  <a href="https://github.com/vibeserve/vibeserve/issues"><img src="https://img.shields.io/github/issues/vibeserve/vibeserve?color=F59E0B" alt="Issues"></a>
</p>

---

VibeServe is a single-binary CLI that turns natural language into a running API server. Describe what you want in plain English (or any language), and VibeServe creates the database schema, API routes, business logic, and seed data — all live, all instantly testable.

No boilerplate. No scaffolding. No framework lock-in. When you're done prototyping, export to a standalone Go server and ship it.

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
| **Export** | `vibeserve export` generates a standalone Go project — Chi router, sqlx, typed models, Dockerfile. |
| **Pluggable LLM** | Works with Claude, OpenAI-compatible APIs (z.ai, x.ai, Groq), and local Ollama models. |
| **Single Binary** | One file. No runtime dependencies. No Docker required. Just download and run. |

## Install

### Quick Install (macOS & Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/vibeserve/vibeserve/main/install.sh | sh
```

This downloads the latest release binary for your platform and puts it in `/usr/local/bin`.

### Go Install

If you have Go installed:

```bash
go install github.com/vibeserve/vibeserve/cmd/vibeserve@latest
```

### Manual Download

Download the binary for your platform from [GitHub Releases](https://github.com/vibeserve/vibeserve/releases) and add it to your PATH.

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
| `vibeserve up` | Start server from existing manifest (no AI) |
| `vibeserve export [dir]` | Export standalone Go project |
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

Graduate from prototype to production:

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
You describe an API
        |
   LLM creates a plan
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

VibeServe stores everything in a `.vibe/` directory:
- `manifest.json` — the API definition (schemas, routes, scripts, seeds)
- `config.yaml` — LLM provider settings
- `state.db` — SQLite database with your data
- `snapshots/` — database snapshots for undo

## Contributing

```bash
git clone https://github.com/vibeserve/vibeserve.git
cd vibeserve
go build -o vibeserve ./cmd/vibeserve
./vibeserve
```

Requires Go 1.23+. No CGO — builds anywhere Go runs.

## Community

- [Issues](https://github.com/vibeserve/vibeserve/issues) — Bug reports and feature requests
- [Discussions](https://github.com/vibeserve/vibeserve/discussions) — Questions and ideas

## License

[MIT](LICENSE) — use it however you want.
