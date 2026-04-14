# Watch Mode — Design Spec

## Goal

`vibeserve watch` runs the API server AND watches the manifest. When the manifest changes (via chat, MCP, direct edit), it auto re-exports the project and generates migration SQL.

## Usage

```bash
vibeserve watch --export ./my-api --format express --db postgres
```

Flags:
- `--export <dir>` — path to the exported project (required)
- `--format <go|express|next>` — export format (default: express)
- `--db <sqlite|postgres>` — database type (default: sqlite)
- `--port <int>` — API server port (default: 8080)
- `--typescript` — generate TypeScript types

## What it does

1. Starts the API server (same as `vibeserve up`)
2. Watches `.vibe/manifest.json` for changes (file system polling)
3. On change:
   a. Reloads the manifest
   b. Diffs against previous manifest
   c. Prints what changed
   d. Re-exports the project to the `--export` directory
   e. Generates migration SQL in `{export}/migrations/` (PostgreSQL only)
   f. Reloads routes on the live server

## Migration Generation

For PostgreSQL exports, generates incremental migration files:

```
my-api/
  migrations/
    001_initial.sql
    002_add_email_verified_to_users.sql
    003_add_notifications_table.sql
```

Each migration contains the ALTER TABLE / CREATE TABLE SQL for the diff. Uses timestamp or sequence numbering.

For SQLite, migrations aren't generated (SQLite is for dev, use the DB directly).

## File Watching

Use simple polling (check file modtime every 1 second). No fsnotify dependency needed — the manifest changes infrequently.

## Architecture

```
vibeserve watch
    ├── HTTP server (serves API at :8080)
    ├── File watcher (polls .vibe/manifest.json every 1s)
    └── On change:
        ├── Reload manifest
        ├── Diff old vs new
        ├── Print changes
        ├── Run export pipeline
        ├── Generate migration SQL
        └── Reload server routes
```

## Implementation

New file: `internal/watch/watch.go` — the watch loop
New file: `internal/watch/migration.go` — migration SQL generation
Modified: `cmd/vibeserve/main.go` — add `watch` subcommand
