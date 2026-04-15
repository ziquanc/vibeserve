# Tunnel Setup — First Time on a Machine

This is the one-time setup needed before `vibeserve live` works. Takes ~10 minutes.

## Prereqs

- macOS, Linux, or Windows
- A Cloudflare account that owns `vibeserve.dev`
- The wildcard DNS already exists in Cloudflare:
  `*.vibeserve.dev CNAME e53bc700-42c9-4592-b5db-4ca504fe391a.cfargotunnel.com (Proxied)`

If you're setting up a fresh tunnel for a different domain, see the
section "Creating a new tunnel" at the bottom.

---

## 1. Install cloudflared

**macOS:**
```bash
brew install cloudflared
cloudflared --version
```

**Linux (Debian/Ubuntu):**
```bash
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o cloudflared.deb
sudo dpkg -i cloudflared.deb
```

**Windows:**
Download the MSI from <https://github.com/cloudflare/cloudflared/releases/latest>.

## 2. Authenticate to your Cloudflare account

```bash
cloudflared tunnel login
```

Opens a browser → pick `vibeserve.dev` → click **Authorize**. This creates
`~/.cloudflared/cert.pem`.

## 3. Get the tunnel credentials

You have two paths:

### Option A — restoring on a new machine (existing tunnel)

Copy `~/.cloudflared/<UUID>.json` from your other machine. See [migrate.md](migrate.md) for the full backup/restore procedure.

### Option B — creating a brand-new tunnel

Skip ahead to "Creating a new tunnel" below.

## 4. Create the config

Copy `config.yml.example` into place:

```bash
cp docs/tunnel/config.yml.example ~/.cloudflared/config.yml
```

Edit it — replace the `<UUID>` placeholders with your real tunnel UUID
(visible in the `.json` filename in `~/.cloudflared/`).

The `ingress` section starts with just a fallback. `vibeserve live` will
add hostname rules above the fallback as needed.

## 5. Validate

```bash
cloudflared tunnel --config ~/.cloudflared/config.yml ingress validate
```

Expected: `OK`. If it errors, check indentation in `config.yml` (YAML is
strict — 2 spaces, no tabs).

## 6. Run the tunnel

```bash
cloudflared tunnel --config ~/.cloudflared/config.yml run vibeserve-main
```

You should see `Registered tunnel connection` ~4 times (one per Cloudflare
edge region). Leave this running — Ctrl-C kills the tunnel.

## 7. Test it

In another terminal:
```bash
python3 -m http.server 9999
```

Then add a temporary ingress rule by editing `~/.cloudflared/config.yml`:
```yaml
ingress:
  - hostname: test.vibeserve.dev
    service: http://localhost:9999
  - service: http_status:404
```

Reload the tunnel without restart:
```bash
kill -HUP $(pgrep -f 'cloudflared tunnel.*run')
```

Open <https://test.vibeserve.dev> — should show Python's directory listing.

If it works, remove the test rule and you're ready for `vibeserve live`.

---

## Optional: run as a background service (recommended for daily use)

Instead of keeping a terminal open with `cloudflared tunnel run`, you can
install it as a launchd service (macOS) or systemd unit (Linux):

```bash
sudo cloudflared service install
```

Now the tunnel auto-starts on boot. Stop / start with:
```bash
sudo launchctl stop com.cloudflare.cloudflared
sudo launchctl start com.cloudflare.cloudflared
```

Logs land in `/Library/Logs/com.cloudflare.cloudflared.{out,err}.log` (macOS).

---

## Creating a new tunnel (advanced)

Only do this once per Cloudflare zone — the `vibeserve.dev` tunnel
(`e53bc700-...`) already exists. This section is for when you're setting
up the tunnel infrastructure on a new domain.

```bash
cloudflared tunnel login              # browser auth, picks the zone
cloudflared tunnel create vibeserve-main
# → outputs the new UUID + path to credentials JSON
```

Then add the wildcard CNAME in Cloudflare dashboard:
- **Type:** CNAME
- **Name:** `*`
- **Target:** `<NEW-UUID>.cfargotunnel.com`
- **Proxy status:** Proxied (orange cloud)

And update `config.yml` with the new UUID.
