# VibeServe Platform Roadmap

From CLI tool to "every business online through AI."

---

## Architecture Overview

```
vibeserve.dev (Cloudflare Pages)
├── Landing page (marketing)
├── Auth (login/register)
├── Dashboard (manage projects)
├── Billing (Stripe)
├── Directory (discover businesses)
└── Business portals (customer-facing pages)

vibeserve CLI / Desktop App (user's machine)
├── Go binary (API engine)
├── SQLite database (user's data)
├── API server (localhost:8080)
├── vibeserve login → authenticates with vibeserve.dev
├── vibeserve live → tunnel to public URL
└── MCP server (for AI agents)

Tunnel Server (Cloudflare Tunnel or custom)
├── Routes *.vibeserve.dev → user's local machine
├── SSL termination
└── Health checks

Shared Services
├── Auth API (verify tokens)
├── Billing API (check subscription)
├── Directory API (register/search businesses)
└── Analytics API (track usage)
```

---

## Phase 1: Web Auth + CLI Login

**Goal:** Users can create an account and authenticate the CLI.

### Web (vibeserve.dev)

```
/login          → email + password login
/register       → create account
/api/auth/*     → auth API endpoints
```

**Tech:** NextAuth.js (or Clerk) on the Next.js site. Store users in a database (Cloudflare D1 or Supabase free tier for the platform DB — NOT the user's data).

**Database (platform only — NOT user data):**
```sql
users (id, email, password_hash, name, plan, created_at)
api_keys (id, user_id, key_hash, name, created_at)
projects (id, user_id, name, subdomain, status, created_at)
subscriptions (id, user_id, plan, stripe_id, status, current_period_end)
```

### CLI

```bash
vibeserve login
→ Opens browser to vibeserve.dev/auth/cli?code=XXXX
→ User logs in on web
→ CLI polls for token
→ Saves token to ~/.vibeserve/credentials.json

vibeserve logout
→ Removes credentials

vibeserve account
→ Shows: email, plan, projects
```

**Files to create:**
- `internal/cloud/auth.go` — token storage, login flow
- `cmd/vibeserve/main.go` — add login/logout/account commands
- `web/src/app/auth/` — login, register pages
- `web/src/app/api/auth/` — auth API routes
- `web/src/app/auth/cli/` — CLI auth callback page

### Deliverables
- [ ] Platform database schema (users, api_keys, projects, subscriptions)
- [ ] Web: register + login pages
- [ ] Web: CLI auth flow (browser-based device auth)
- [ ] CLI: `vibeserve login` command
- [ ] CLI: token storage in ~/.vibeserve/
- [ ] CLI: `vibeserve account` command

---

## Phase 2: Project Dashboard

**Goal:** Logged-in users can manage their VibeServe projects from the web.

### Web (vibeserve.dev)

```
/dashboard                → list all projects
/dashboard/new            → create new project (pick template or describe)
/dashboard/[id]           → project detail (tables, routes, status)
/dashboard/[id]/settings  → project settings (name, subdomain)
```

**Dashboard shows:**
- Project name + status (running / stopped / live)
- Table count, route count
- Public URL (if live)
- Quick actions: Go Live, Export, Delete

**No data management here** — the user's data stays on their machine. The dashboard just shows metadata synced from the CLI.

### CLI sync

When the user runs `vibeserve` with a logged-in account, it syncs project metadata to the platform:

```bash
vibeserve
→ Detects logged-in user
→ Syncs: project name, table count, route count, manifest summary
→ Shows on web dashboard
```

**Files to create:**
- `web/src/app/dashboard/` — dashboard pages
- `web/src/app/api/projects/` — projects API
- `internal/cloud/sync.go` — sync project metadata to platform
- `internal/cloud/client.go` — HTTP client for platform API

### Deliverables
- [ ] Web: dashboard page (list projects)
- [ ] Web: project detail page
- [ ] Web: create project page (with template selection)
- [ ] API: CRUD for projects
- [ ] CLI: auto-sync project metadata when logged in
- [ ] CLI: `vibeserve projects` command (list)

---

## Phase 3: Tunnel (Go Live)

**Goal:** One command makes a local API publicly accessible.

### How it works

```bash
vibeserve live
→ Checks: logged in? ✓
→ Checks: subscription? (free gets 1 project)
→ Starts cloudflared tunnel
→ Routes: coffee-shop.vibeserve.dev → localhost:8080
→ Registers in directory
→ Generates .well-known/vibeserve.json
→ Shows: "Your API is live at https://coffee-shop.vibeserve.dev"
```

### Tech options for tunnel

**Option A: Cloudflare Tunnel (recommended)**
- Free, fast, reliable
- User installs cloudflared (or we bundle it)
- We manage DNS for *.vibeserve.dev on Cloudflare
- Each project gets a subdomain

**Option B: Custom tunnel server**
- More control but more infra to manage
- WebSocket-based reverse proxy
- We run the server, user connects

**Recommend Option A** — Cloudflare Tunnel is free and handles SSL, DDoS, caching.

### .well-known/vibeserve.json

Auto-generated when `vibeserve live` runs:

```json
{
  "name": "Coffee Corner",
  "type": "restaurant",
  "location": {"city": "Kuala Lumpur", "country": "MY"},
  "capabilities": ["menu", "ordering", "delivery"],
  "endpoints": {
    "menu": "GET /menu",
    "order": "POST /orders",
    "status": "GET /orders/:id"
  },
  "openapi": "/openapi.yaml"
}
```

The CLI generates this from the manifest automatically.

### Files to create
- `internal/cloud/tunnel.go` — start/stop cloudflared tunnel
- `internal/cloud/wellknown.go` — generate .well-known/vibeserve.json
- `cmd/vibeserve/main.go` — add `live` command
- DNS setup for *.vibeserve.dev on Cloudflare

### Deliverables
- [ ] CLI: `vibeserve live` command
- [ ] CLI: `vibeserve live --stop` command
- [ ] Cloudflare DNS setup for *.vibeserve.dev
- [ ] Auto-generate .well-known/vibeserve.json from manifest
- [ ] Register project as "live" in platform API
- [ ] Web dashboard shows "Live" status + public URL

---

## Phase 4: Billing (Stripe)

**Goal:** Subscription management for paid features.

### Plans

```
Free ($0):
  - 1 project
  - Local only (no tunnel)
  - Export to code
  - MCP server
  - All templates

Pro ($19/mo):
  - Unlimited projects
  - 1 live tunnel (public URL)
  - Custom subdomain
  - Daily snapshots (auto backup manifest + DB)
  - AI discoverable in directory
  - Priority support

Business ($49/mo):
  - Everything Pro
  - 5 live tunnels
  - Custom domain
  - Analytics (API call counts, popular endpoints)
  - Multiple locations
  - Team members
```

### Tech
- Stripe Checkout for subscription
- Stripe Webhooks for plan changes
- CLI checks plan before enabling paid features

### Files to create
- `web/src/app/api/billing/` — Stripe checkout, webhooks
- `web/src/app/dashboard/billing/` — manage subscription page
- `internal/cloud/billing.go` — check subscription status from CLI

### Deliverables
- [ ] Stripe account setup
- [ ] Web: pricing page → Stripe Checkout flow
- [ ] Web: billing management page
- [ ] API: webhook handler for subscription events
- [ ] CLI: subscription check before `vibeserve live`
- [ ] CLI: `vibeserve upgrade` opens billing page

---

## Phase 5: Directory + Discovery

**Goal:** AI agents can discover and interact with live businesses.

### Web directory

```
vibeserve.dev/directory                    → browse all live businesses
vibeserve.dev/directory?type=restaurant    → filter by type
vibeserve.dev/directory?city=KL            → filter by location
vibeserve.dev/b/coffee-corner              → business profile page
```

### Business profile page

```
vibeserve.dev/b/coffee-corner

  Coffee Corner
  Restaurant — Kuala Lumpur

  API: coffee-corner.vibeserve.dev
  Docs: coffee-corner.vibeserve.dev/_swagger

  Capabilities: Menu, Ordering, Delivery
  
  [View Menu]  [API Documentation]
```

### Directory API (for AI agents)

```
GET api.vibeserve.dev/v1/directory
  ?type=restaurant
  &city=KL
  &q=coffee

→ [
    {
      "name": "Coffee Corner",
      "url": "https://coffee-corner.vibeserve.dev",
      "type": "restaurant",
      "city": "Kuala Lumpur",
      "capabilities": ["menu", "ordering", "delivery"]
    }
  ]
```

### Network MCP

AI agents use one MCP server to discover ALL businesses:

```json
{
  "mcpServers": {
    "vibeserve-network": {
      "command": "vibeserve",
      "args": ["network"]
    }
  }
}
```

Tools:
- `discover_businesses(type, location, query)` → search directory
- `get_business_info(url)` → capabilities, endpoints
- `call_api(url, method, path, body)` → interact with business API

### Files to create
- `web/src/app/directory/` — directory browse page
- `web/src/app/b/[slug]/` — business profile page
- `web/src/app/api/directory/` — directory API
- `internal/cloud/directory.go` — register/update business listing
- `cmd/vibeserve/main.go` — add `network` command (MCP)

### Deliverables
- [ ] Web: directory browse page with search/filter
- [ ] Web: business profile page
- [ ] API: directory search endpoint
- [ ] CLI: auto-register in directory when going live
- [ ] CLI: business metadata (name, type, location) setup
- [ ] MCP: `vibeserve network` with discovery tools

---

## Phase 6: Customer Portal

**Goal:** Each live business gets a customer-facing page.

```
coffee-corner.vibeserve.dev
→ Customer can browse menu
→ Customer can place orders
→ Customer can track orders
→ Powered by the business's own API
```

### How it works

The portal is auto-generated from the manifest — similar to how we generate Next.js admin panels, but for customers:

- Restaurant → menu + ordering UI
- Salon → service list + booking UI
- Clinic → appointment booking UI

### Tech

Either:
- **Server-rendered pages** on the tunnel (Go serves HTML)
- **Static pages** generated and hosted on Cloudflare Pages

Recommend: Go serves the portal at the root path. The API is at `/api/*`. The portal is at `/*`.

### Files to create
- `internal/portal/` — customer-facing HTML templates
- `internal/portal/restaurant.go` — restaurant portal
- `internal/portal/booking.go` — booking portal (salon, clinic, etc.)
- Integrate into the Go binary's HTTP server

### Deliverables
- [ ] Portal template: restaurant (menu + order)
- [ ] Portal template: booking (services + appointments)
- [ ] Portal template: generic (data display + contact)
- [ ] Portal auto-selected based on manifest business type
- [ ] Mobile-responsive design
- [ ] Order/booking forms that call the business's API

---

## Phase 7: Desktop App (Tauri)

**Goal:** Business owners who don't use CLI can install a desktop app.

### Tech
- Tauri (Rust wrapper, small binary)
- Embeds the Go binary
- Opens web dashboard in a native window
- System tray icon to keep running in background

### What it looks like
```
Download VibeServe.dmg (macOS) / VibeServe.exe (Windows)
→ Install
→ Open
→ "Describe your business" text box
→ Click Generate
→ See tables, routes, status
→ Click "Go Live"
→ Business is online
```

### Deliverables
- [ ] Tauri project setup
- [ ] Embed Go binary
- [ ] Native window with web dashboard
- [ ] System tray icon
- [ ] Auto-start on boot (optional)
- [ ] macOS + Windows installers

---

## Implementation Order

```
Phase 1: Auth              ✅ shipped in v0.5.0
Phase 2: Dashboard         ✅ shipped in v0.5.0
Phase 3: Tunnel            ✅ shipped in v0.6.0 — try: vibeserve live
Phase 4: Billing           — deferred
Phase 5: Directory         — planned
Phase 6: Customer Portal   — planned
Phase 7: Desktop App       — planned
```

Phases 4-7 scope and ordering are under review after usage signal from Phases 1-3.

---

## Tech Stack Summary

| Component | Tech | Why |
|-----------|------|-----|
| Landing page | Next.js on Cloudflare Pages | Free, fast, already built |
| Auth | NextAuth.js or Clerk | Standard, secure |
| Platform DB | Cloudflare D1 (SQLite) or Supabase | Free tier, simple |
| Billing | Stripe | Industry standard |
| Tunnel | Cloudflare Tunnel (cloudflared) | Free, reliable |
| DNS | Cloudflare | Already using for Pages |
| Directory API | Next.js API routes | Same codebase |
| Customer portal | Go (embedded in binary) | Same binary, no extra hosting |
| Desktop app | Tauri | Small, native, cross-platform |
| CLI | Go (existing) | Already built |
