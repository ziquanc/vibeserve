# Migrating the Tunnel to Another Machine

The tunnel UUID never changes. The wildcard CNAME never changes.
Switching machines is just moving the credentials.

## Important constraints

- **Only ONE machine can run the tunnel at a time.** If you start
  `cloudflared tunnel run` on two machines, you'll get split-brain
  routing and unpredictable failures. Stop the old machine before
  starting the new.
- **Lose the credentials, lose the tunnel.** If `<UUID>.json` is gone
  with no backup, you'll have to delete the tunnel and create a new
  one (which means a new UUID, which means updating the wildcard CNAME).

## Step 1 — Back up on the old machine

```bash
tar czf ~/Desktop/cloudflared-backup-$(date +%Y%m%d).tar.gz \
  -C "$HOME" .cloudflared
```

The archive contains:
- `cert.pem` — Cloudflare zone-level account auth
- `e53bc700-42c9-4592-b5db-4ca504fe391a.json` — tunnel credentials
- `config.yml` — your ingress rules

## Step 2 — Store the backup somewhere safe

**Recommended:** drop `cloudflared-backup-YYYYMMDD.tar.gz` into 1Password,
your password manager's file attachments, or an encrypted USB.

**Do NOT:**
- Commit it to git
- Email it to yourself unencrypted
- Leave it in `~/Desktop` long-term

## Step 3 — Stop the old tunnel

If running in foreground, Ctrl-C the cloudflared process.

If running as a service:
```bash
# macOS
sudo launchctl stop com.cloudflare.cloudflared

# Linux (systemd)
sudo systemctl stop cloudflared
```

Verify nothing is connected:
```bash
cloudflared tunnel info vibeserve-main
# → "ACTIVE CONNECTORS: 0" means it's safe to start elsewhere
```

## Step 4 — Restore on the new machine

```bash
# Install cloudflared
brew install cloudflared    # macOS
# or apt / yum / msi for Linux/Windows

# Restore the backup
tar xzf cloudflared-backup-YYYYMMDD.tar.gz -C "$HOME"
ls -la ~/.cloudflared
# → should see cert.pem, <UUID>.json, config.yml

# Start the tunnel
cloudflared tunnel --config ~/.cloudflared/config.yml run vibeserve-main
```

The wildcard DNS already points to your tunnel UUID, so within seconds of
this command, `*.vibeserve.dev` URLs are live again from the new machine.

## Step 5 — Verify

```bash
# In another terminal on the new machine, point an ingress at a quick test
echo '
ingress:
  - hostname: test.vibeserve.dev
    service: http://localhost:9999
  - service: http_status:404
' >> ~/.cloudflared/config.yml

python3 -m http.server 9999 &
kill -HUP $(pgrep -f 'cloudflared tunnel.*run')

curl https://test.vibeserve.dev
# → should return Python's directory listing HTML
```

Clean up the test rule when done.

## Recovery: lost credentials

If you don't have `<UUID>.json` anymore:

```bash
# Delete the dead tunnel
cloudflared tunnel delete vibeserve-main

# Create a new one
cloudflared tunnel create vibeserve-main
# → outputs a NEW UUID and credentials path
```

Then update the wildcard CNAME in Cloudflare dashboard to point to the
new `<NEW-UUID>.cfargotunnel.com`. Update `~/.cloudflared/config.yml`
with the new UUID. Back up immediately this time.
