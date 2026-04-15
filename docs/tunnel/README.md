# VibeServe Tunnel — Operator Guide

This is the **single shared tunnel** that fronts every `vibeserve live` URL
under `*.vibeserve.dev`. As of v0.6, only the platform operator (Kent) runs
this tunnel; future versions may switch to per-user tunnels.

## Architecture (Model A: shared tunnel)

```
Browser
   │
   ▼
*.vibeserve.dev  ──[Cloudflare DNS wildcard CNAME]──▶  <UUID>.cfargotunnel.com
                                                            │
                                                            ▼
                                              Operator's machine running:
                                                cloudflared tunnel run
                                                            │
                                  reads ~/.cloudflared/config.yml
                                                            │
                              ingress rules dispatch by hostname
                                                            │
                          coffee.vibeserve.dev → http://localhost:8080
                          salon.vibeserve.dev  → http://localhost:8081
                                ...
```

The tunnel UUID never changes. The wildcard CNAME points to the tunnel,
not to a particular machine. As long as ONE machine is running
`cloudflared tunnel run vibeserve-main`, every subdomain works.

## Files in this folder

| File | Purpose |
|---|---|
| `README.md` | This file — overview + commands |
| `setup.md` | First-time setup on a new machine (7 steps) |
| `migrate.md` | Move the tunnel to a different machine without downtime |
| `config.yml.example` | Sample config — copy to `~/.cloudflared/config.yml` and edit |

## Daily commands

```bash
# Start the tunnel (foreground, blocks)
cloudflared tunnel --config ~/.cloudflared/config.yml run vibeserve-main

# Validate config before reload
cloudflared tunnel --config ~/.cloudflared/config.yml ingress validate

# Reload config without restart (after vibeserve live edits the file)
kill -HUP $(pgrep -f 'cloudflared tunnel.*run')

# Show what's currently routed
cat ~/.cloudflared/config.yml

# List all tunnels you own
cloudflared tunnel list
```

## Files cloudflared creates

All in `~/.cloudflared/`:

| File | What it is | Sensitive? |
|---|---|---|
| `cert.pem` | Cloudflare account auth (zone-level) | **Yes** — back up |
| `<UUID>.json` | Tunnel-specific credentials | **Yes** — back up |
| `config.yml` | Ingress rules — managed by `vibeserve live` | No (recreatable) |

## See also

- [`setup.md`](setup.md) — first-time setup
- [`migrate.md`](migrate.md) — switching machines
- [Cloudflare Tunnel docs](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
