# Auth System — Design Spec

## Goal

Auto-generate a complete auth system when a `users` table has a `password_hash` column. Includes registration, login, JWT tokens with refresh, password reset, email verification, and role-based middleware.

## Auto-detection

When any schema has a `users` table (or similar — detected by having `email` + `password_hash` columns), VibeServe auto-generates auth infrastructure.

## Tables auto-added

If not already present in the manifest:

```json
{
  "table": "refresh_tokens",
  "columns": [
    {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
    {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
    {"name": "token", "type": "TEXT", "required": true, "unique": true},
    {"name": "expires_at", "type": "DATETIME", "required": true},
    {"name": "revoked", "type": "BOOLEAN", "default": false}
  ]
},
{
  "table": "password_resets",
  "columns": [
    {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
    {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
    {"name": "token", "type": "TEXT", "required": true, "unique": true},
    {"name": "expires_at", "type": "DATETIME", "required": true},
    {"name": "used", "type": "BOOLEAN", "default": false}
  ]
}
```

## Routes auto-generated

| Route | Method | Auth | Description |
|-------|--------|------|-------------|
| `/auth/register` | POST | No | Create user with hashed password, return access + refresh tokens |
| `/auth/login` | POST | No | Verify password, return access + refresh tokens |
| `/auth/refresh` | POST | No | Exchange refresh token for new access + refresh tokens (rotation) |
| `/auth/logout` | POST | Yes | Revoke the refresh token |
| `/me` | GET | Yes | Get current user profile from JWT |
| `/me` | PUT | Yes | Update current user profile (name, bio, etc. — NOT password/email) |
| `/auth/forgot-password` | POST | No | Generate password reset token (returned in response — no email) |
| `/auth/reset-password` | POST | No | Verify reset token + set new password |
| `/auth/verify-email` | POST | Yes | Mark current user's email as verified |

## JWT Tokens

- **Access token**: 15 min expiry. Payload: `{user_id, role, email}`. Signed with JWT secret.
- **Refresh token**: random UUID, 7 day expiry. Stored in `refresh_tokens` table.
- **Token rotation**: on refresh, old token is revoked, new pair issued.
- **Secret**: auto-generated random string on first run, saved to `.vibe/config.yaml` as `jwt_secret`. In production (export), requires `JWT_SECRET` env var.

## Password Hashing

- bcrypt via Go's `golang.org/x/crypto/bcrypt` (cost factor 10)
- Exposed in Tengo scripts as:
  - `crypto.hash_password(plain)` → returns bcrypt hash string
  - `crypto.verify_password(plain, hash)` → returns true/false
- These are added to the existing `crypto` stdlib module in the runtime

## Runtime Auth Middleware

`request.auth()` in Tengo scripts:
1. Reads `Authorization: Bearer <token>` header
2. Verifies JWT signature and expiry
3. Returns `{user_id: int, role: string, email: string}` or `undefined`
4. Does NOT block the request — scripts decide what to do with `undefined`

This already exists in the runtime but currently returns empty map. Needs to be wired to actual JWT verification.

## Generated Script Logic

### POST /auth/register
```
body := request.body()
if body.email == undefined || body.password == undefined {
  response.fail(400, "email and password required")
}
existing := db.query_one("SELECT id FROM users WHERE email = ? AND deleted_at IS NULL", [body.email])
if existing != undefined {
  response.fail(409, "email already registered")
}
hashed := crypto.hash_password(body.password)
user := db.insert("users", {email: body.email, password_hash: hashed, name: body.name, role: "user"})
tokens := auth.generate_tokens(user.id, user.role, user.email)
response.json({user: {id: user.id, email: user.email, name: user.name, role: user.role}, access_token: tokens.access_token, refresh_token: tokens.refresh_token}, 201)
```

### POST /auth/login
```
body := request.body()
user := db.query_one("SELECT * FROM users WHERE email = ? AND deleted_at IS NULL", [body.email])
if user == undefined {
  response.fail(401, "invalid credentials")
}
if !crypto.verify_password(body.password, user.password_hash) {
  response.fail(401, "invalid credentials")
}
tokens := auth.generate_tokens(user.id, user.role, user.email)
response.json({user: {id: user.id, email: user.email, name: user.name, role: user.role}, access_token: tokens.access_token, refresh_token: tokens.refresh_token})
```

### POST /auth/refresh
```
body := request.body()
stored := db.query_one("SELECT * FROM refresh_tokens WHERE token = ? AND revoked = 0 AND expires_at > date.now()", [body.refresh_token])
if stored == undefined {
  response.fail(401, "invalid or expired refresh token")
}
db.query("UPDATE refresh_tokens SET revoked = 1 WHERE id = ?", [stored.id])
user := db.query_one("SELECT * FROM users WHERE id = ?", [stored.user_id])
tokens := auth.generate_tokens(user.id, user.role, user.email)
response.json({access_token: tokens.access_token, refresh_token: tokens.refresh_token})
```

### GET /me
```
user_auth := request.auth()
if user_auth == undefined {
  response.fail(401, "authentication required")
}
user := db.query_one("SELECT id, email, name, role, created_at FROM users WHERE id = ? AND deleted_at IS NULL", [user_auth.user_id])
if user == undefined {
  response.fail(404, "user not found")
}
response.json(user)
```

## New Tengo Stdlib: `auth` module

Add `auth` module alongside existing `db`, `request`, `response`, `date`, `crypto`, `log`:

- `auth.generate_tokens(user_id, role, email)` → returns `{access_token, refresh_token}`
  - Signs JWT access token with secret
  - Creates refresh token (UUID) in `refresh_tokens` table
  - Returns both

## Config

Add to `.vibe/config.yaml`:
```yaml
jwt_secret: "auto-generated-random-string"
jwt_access_expiry: "15m"
jwt_refresh_expiry: "168h"  # 7 days
```

## Export Integration

Express export:
- Auth routes use the existing `auth.js` middleware (already generates JWT + bcrypt)
- Refresh tokens table + password resets table included in schema.sql
- `.env.example` already has JWT_SECRET

Next.js export:
- Add login + register pages
- Auth context provider with token storage
- API client includes auth headers automatically

## LLM Prompt Update

Teach the LLM:
- Auth routes are AUTO-GENERATED when users table has password_hash
- Do NOT manually create /auth/register or /auth/login routes
- Use `request.auth()` in scripts to check authentication
- Use `auth.role` to check roles

## Implementation Files

### New files
- `internal/auth/auth.go` — JWT signing/verification, password hashing, token generation
- `internal/auth/auth_test.go` — tests
- `internal/manifest/auth.go` — DetectAuth(), GenerateAuthRoutes(), GenerateAuthTables()

### Modified files
- `internal/engine/engine.go` — inject auth tables/routes in applyManifest (like state machines)
- `internal/runtime/stdlib.go` — add `auth` and enhanced `crypto` modules to Tengo
- `internal/llm/provider.go` — update prompt
- `internal/config/config.go` — add jwt_secret field
