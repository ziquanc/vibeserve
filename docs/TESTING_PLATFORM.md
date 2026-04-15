# Testing VibeServe Platform Auth

End-to-end walkthrough for Phase 1: verify a user can register on the web,
connect the CLI via browser-based device auth, and manage their session
with `login` / `logout` / `account`.

Takes about 2 minutes.

---

## Prereqs

- Go 1.22+
- Node 20+
- Two terminal windows
- A web browser

Both repos checked out side-by-side:

```
~/Documents/Projects/
  vibeserve/         ← this CLI repo (public)
  vibeserve-web/     ← platform web app (private)
```

---

## Setup

### Terminal 1 — Web app

```bash
cd ~/Documents/Projects/vibeserve-web
npm install          # first time only
npm run dev
```

Wait until you see `▲ Next.js ... - Local: http://localhost:3000`.

### Terminal 2 — CLI

```bash
cd ~/Documents/Projects/vibeserve
go build -o /tmp/vibeserve ./cmd/vibeserve
```

The CLI defaults to `PlatformURL = "http://localhost:3000"`
(see `internal/cloud/login.go`), so no config needed for local dev.

---

## 1. Register an account on the web

Open <http://localhost:3000/auth/register> in your browser.

Fill in:

- **Name:** anything (e.g. `Kent`)
- **Email:** anything (e.g. `me@test.local`)
- **Password:** any string

Click **Create account**.

**Expected:** page redirects to `/` (landing page). A JWT is stored in
`localStorage`.

> Users are persisted to `.data/db.json` in the web repo (JSON file, not a real
> DB yet). They survive `npm run dev` restarts. To reset, delete `.data/`.

---

## 2. Start CLI login

In Terminal 2:

```bash
/tmp/vibeserve login
```

**Expected CLI output:**

```
  Opening browser for login...
  If it doesn't open, visit:
  http://localhost:3000/auth/device?code=<uuid>

  Waiting for login....
```

Your browser should open the device auth page automatically. If not,
click the URL in the terminal.

---

## 3. Authorize the CLI in the browser

On the **Connect your CLI** page:

- **Email:** the one you registered in step 1
- **Password:** the one you registered in step 1

Click **Connect CLI**.

**Expected:** page flips to a green checkmark with "CLI connected!
You can close this tab."

---

## 4. CLI detects the authorization

Switch back to Terminal 2. Within ~2 seconds the CLI should print:

```
  Logged in as me@test.local (free plan)
```

Credentials are saved to `~/.vibeserve/credentials.json` (mode `0600`).

---

## 5. Verify credentials

```bash
cat ~/.vibeserve/credentials.json
```

**Expected:** a JSON object with three fields:

```json
{
  "token": "eyJhbGciOiJIUzI1NiJ9...",
  "email": "me@test.local",
  "plan": "free"
}
```

---

## 6. `vibeserve account`

```bash
/tmp/vibeserve account
```

**Expected:**

```
  Email: me@test.local
  Plan:  free
```

---

## 7. `vibeserve logout`

```bash
/tmp/vibeserve logout
```

**Expected:**

```
  Logged out.
```

Then verify the file is gone:

```bash
ls ~/.vibeserve/
```

Should be empty (no `credentials.json`).

Running `/tmp/vibeserve account` again should now print
`Not logged in. Run 'vibeserve login' to connect.`

---

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| CLI times out after 5 min | Web dev server not reachable, or you never submitted the `/auth/device` form |
| "Invalid email or password" on device page | User not registered (do step 1) or web server was restarted (in-memory store wiped) |
| Browser didn't open automatically | Click the URL printed in the terminal |
| Connection refused | `npm run dev` isn't running, or it bound to a different port |
| Want to test against prod | Change `PlatformURL` in `internal/cloud/login.go` and rebuild |

---

## What this proves

- Web register + login + JWT signing works (`web/src/lib/auth.ts`)
- Device code flow works: authorize → poll → redeem (one-time use)
- CLI polls correctly every 2s for up to 5 minutes
- Credentials roundtrip through `~/.vibeserve/credentials.json`
- `account` / `logout` behave correctly when logged in and not logged in

---

# Phase 2: Dashboard + Sync

Assumes you've completed the Phase 1 auth walkthrough above and are logged in
(`~/.vibeserve/credentials.json` exists).

## 1. Sync a project from the CLI

```bash
mkdir -p /tmp/sync-demo/.vibe
cat > /tmp/sync-demo/.vibe/manifest.json <<'EOF'
{
  "version": "1",
  "name": "Coffee Corner",
  "description": "Test project for Phase 2 walkthrough",
  "schemas": [
    {"table":"menu","columns":[{"name":"id","type":"integer","primary":true}]},
    {"table":"orders","columns":[{"name":"id","type":"integer","primary":true}]}
  ],
  "routes": [
    {"method":"GET","path":"/menu","script":""},
    {"method":"POST","path":"/orders","script":""}
  ],
  "scripts": [],
  "seeds": []
}
EOF

# Minimal config so dev doesn't prompt for LLM setup
cat > /tmp/sync-demo/.vibe/config.yaml <<'EOF'
server:
  host: localhost
  port: 8765
provider:
  name: anthropic
  model: claude-sonnet-4-5-20250929
  api_key: "SKIP"
EOF

cd /tmp/sync-demo
/tmp/vibeserve dev &
sleep 5
kill %1 2>/dev/null
wait %1 2>/dev/null

cat .vibe/project.json
# Expect: { "id": "..." }
```

## 2. List via CLI

```bash
/tmp/vibeserve projects
```

Expect: a row showing "Coffee Corner", status "running", 2/2 tables/routes.

## 3. View in dashboard

Open http://localhost:3000/dashboard in your browser — Coffee Corner appears as a card.

Click the card → detail page shows:
- Status: running
- Tables: 2
- Routes: 2
- Last sync: a recent timestamp

## 4. Cross-surface delete

On the detail page, click **Delete project**. Confirm the dialog.

Back in the terminal:

```bash
/tmp/vibeserve projects
```

Expect: "No projects yet. Run 'vibeserve dev' in a project dir to sync."

## 5. Re-sync creates a new project

```bash
cd /tmp/sync-demo
# Clean the local cache first so CLI re-creates rather than PATCHes a stale ID:
rm .vibe/project.json
/tmp/vibeserve dev &
sleep 5
kill %1 2>/dev/null
wait %1 2>/dev/null
```

Refresh the dashboard — Coffee Corner is back with a new ID.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `vibeserve projects` shows nothing after `vibeserve dev` | Sync is async (goroutine) — give it a few extra seconds, or check the log for "project sync failed" |
| Dashboard returns 401 repeatedly | Token expired — clear `~/.vibeserve/credentials.json` and re-login |
| Dev server won't start | Check `.vibe/config.yaml` has a provider.api_key (use "SKIP" if you just want to test sync, not LLM features) |
| `.data/db.json` missing users on restart | Confirm `.data/` is gitignored in the web repo; the file persists across restarts but not across `.data/` deletion |

---

# Phase 3: Go Live (Tunnel)

Assumes:
- You completed the Phase 1 + 2 walkthroughs above
- The `vibeserve.dev` tunnel is set up (see `docs/tunnel/setup.md`)
- `cloudflared` is installed and is **not** currently running

## 1. Make sure the tunnel infrastructure is ready

```bash
# Validate config
cloudflared tunnel --config ~/.cloudflared/config.yml ingress validate
# Expected: OK

# Verify cloudflared isn't already running
pgrep -fl 'cloudflared tunnel.*run' && echo "stop it first" || echo "OK"
```

## 2. Start the API server

```bash
cd /tmp/sync-demo  # the project from Phase 2 walkthrough
/tmp/vibeserve dev &
sleep 3
```

This also re-syncs the project to the platform (status=running).

## 3. Take it live

In another terminal:

```bash
cd /tmp/sync-demo
/tmp/vibeserve live
```

You should see cloudflared startup logs followed by:

```
  ✦ Live at https://coffee-corner.vibeserve.dev
  Press Ctrl-C to stop.
```

(Subdomain comes from kebab-case of your project name. Override with `--subdomain mycoffee`.)

## 4. Verify the public URL works

In a third terminal:

```bash
curl https://coffee-corner.vibeserve.dev/menu
# Should return whatever your local API returns at GET /menu
```

## 5. Verify dashboard updated

Open http://localhost:3000/dashboard:
- The project card shows a green dot + the live URL
- Click the card → detail page has a prominent green "Live" panel + Copy button

## 6. Verify .well-known descriptor

```bash
curl -s https://coffee-corner.vibeserve.dev/.well-known/vibeserve.json | python3 -m json.tool
```

Expected output: JSON with `name`, `description`, `endpoints` (one per route), `openapi`.

## 7. Stop and verify cleanup

In the `vibeserve live` terminal, Ctrl-C. You should see:

```
  Tunnel stopped.
```

Verify cleanup:

```bash
# ingress rule removed from config.yml
grep coffee-corner ~/.cloudflared/config.yml || echo "OK: ingress cleaned up"

# Dashboard back to status=running
# (refresh the dashboard tab — the green dot should be gone, status badge is "running")
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `cloudflared not found in PATH` | `brew install cloudflared` |
| `cloudflared config not found` | See `docs/tunnel/setup.md` |
| Live URL returns Cloudflare error 1033 | cloudflared subprocess died — check `vibeserve live` output |
| Live URL returns 502 | API server isn't running on the configured port |
| Live URL returns 404 | Hit a path that doesn't match a route in your manifest |
| `tunnel already has a connection` | Another cloudflared is running externally; stop it first (`pkill -f 'cloudflared tunnel.*run'`) |
| Hitting Ctrl-C twice escalates to SIGKILL | Working as designed — the second signal forces immediate exit if cloudflared stalls |
