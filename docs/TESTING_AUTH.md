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

> The web uses in-memory user storage (`web/src/lib/db.ts`). If you restart
> `npm run dev`, all registered users are wiped — you'll need to re-register.

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
