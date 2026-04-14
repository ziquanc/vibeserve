package manifest

import "strings"

// DetectAuth returns true if any schema has a table named "users" (case-insensitive)
// AND that table has a column named "password_hash" or "password".
func DetectAuth(schemas []Schema) bool {
	for _, s := range schemas {
		if !strings.EqualFold(s.Table, "users") {
			continue
		}
		for _, c := range s.Columns {
			if c.Name == "password_hash" || c.Name == "password" {
				return true
			}
		}
	}
	return false
}

// GenerateAuthTables returns the supporting schemas required for authentication:
// refresh_tokens and password_resets.
func GenerateAuthTables() []Schema {
	return []Schema{
		{
			Table: "refresh_tokens",
			Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "user_id", Type: "INTEGER", Required: true, References: "users.id"},
				{Name: "token", Type: "TEXT", Required: true, Unique: true},
				{Name: "expires_at", Type: "DATETIME", Required: true},
				{Name: "revoked", Type: "BOOLEAN", Default: false},
			},
		},
		{
			Table: "password_resets",
			Columns: []Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "user_id", Type: "INTEGER", Required: true, References: "users.id"},
				{Name: "token", Type: "TEXT", Required: true, Unique: true},
				{Name: "expires_at", Type: "DATETIME", Required: true},
				{Name: "used", Type: "BOOLEAN", Default: false},
			},
		},
	}
}

// GenerateAuthRoutes returns the 9 standard auth routes and their corresponding scripts.
func GenerateAuthRoutes(usersTable string) ([]Route, []Script) {
	routes := []Route{
		{Path: "/auth/register", Method: "POST", Description: "Register a new user", Script: "auth_register", ResponseType: "object"},
		{Path: "/auth/login", Method: "POST", Description: "Log in with email and password", Script: "auth_login", ResponseType: "object"},
		{Path: "/auth/refresh", Method: "POST", Description: "Refresh access token", Script: "auth_refresh", ResponseType: "object"},
		{Path: "/auth/logout", Method: "POST", Description: "Log out and revoke refresh token", Script: "auth_logout", ResponseType: "object"},
		{Path: "/me", Method: "GET", Description: "Get current user profile", Script: "auth_me", ResponseType: "object"},
		{Path: "/me", Method: "PUT", Description: "Update current user profile", Script: "auth_update_me", ResponseType: "object"},
		{Path: "/auth/forgot-password", Method: "POST", Description: "Request a password reset token", Script: "auth_forgot_password", ResponseType: "object"},
		{Path: "/auth/reset-password", Method: "POST", Description: "Reset password with token", Script: "auth_reset_password", ResponseType: "object"},
		{Path: "/auth/verify-email", Method: "POST", Description: "Verify email address", Script: "auth_verify_email", ResponseType: "object"},
	}

	scripts := []Script{
		{
			Name: "auth_register",
			Code: `body := request.body()
if body.email == undefined || body.password == undefined {
  response.fail(400, "email and password required")
}
existing := db.query_one("SELECT id FROM ` + usersTable + ` WHERE email = ? AND deleted_at IS NULL", [body.email])
if existing != undefined {
  response.fail(409, "email already registered")
}
hashed := crypto.hash_password(body.password)
user := db.insert("` + usersTable + `", {email: body.email, password_hash: hashed, name: body.name, role: "user"})
tokens := auth.generate_tokens(user.id, user.role, user.email)
response.json({user: {id: user.id, email: user.email, name: user.name, role: user.role}, access_token: tokens.access_token, refresh_token: tokens.refresh_token}, 201)`,
		},
		{
			Name: "auth_login",
			Code: `body := request.body()
if body.email == undefined || body.password == undefined {
  response.fail(400, "email and password required")
}
user := db.query_one("SELECT * FROM ` + usersTable + ` WHERE email = ? AND deleted_at IS NULL", [body.email])
if user == undefined {
  response.fail(401, "invalid credentials")
}
if !crypto.verify_password(body.password, user.password_hash) {
  response.fail(401, "invalid credentials")
}
tokens := auth.generate_tokens(user.id, user.role, user.email)
response.json({user: {id: user.id, email: user.email, name: user.name, role: user.role}, access_token: tokens.access_token, refresh_token: tokens.refresh_token})`,
		},
		{
			Name: "auth_refresh",
			Code: `body := request.body()
if body.refresh_token == undefined {
  response.fail(400, "refresh_token required")
}
stored := db.query_one("SELECT * FROM refresh_tokens WHERE token = ? AND revoked = 0", [body.refresh_token])
if stored == undefined {
  response.fail(401, "invalid or expired refresh token")
}
db.query("UPDATE refresh_tokens SET revoked = 1 WHERE id = ?", [stored.id])
user := db.query_one("SELECT * FROM ` + usersTable + ` WHERE id = ? AND deleted_at IS NULL", [stored.user_id])
if user == undefined {
  response.fail(401, "user not found")
}
tokens := auth.generate_tokens(user.id, user.role, user.email)
response.json({access_token: tokens.access_token, refresh_token: tokens.refresh_token})`,
		},
		{
			Name: "auth_logout",
			Code: `body := request.body()
if body.refresh_token == undefined {
  response.fail(400, "refresh_token required")
}
db.query("UPDATE refresh_tokens SET revoked = 1 WHERE token = ?", [body.refresh_token])
response.json({message: "logged out"})`,
		},
		{
			Name: "auth_me",
			Code: `me := request.auth()
if me == undefined {
  response.fail(401, "authentication required")
}
user := db.query_one("SELECT id, email, name, role, created_at FROM ` + usersTable + ` WHERE id = ? AND deleted_at IS NULL", [me.user_id])
if user == undefined {
  response.fail(404, "user not found")
}
response.json(user)`,
		},
		{
			Name: "auth_update_me",
			Code: `me := request.auth()
if me == undefined {
  response.fail(401, "authentication required")
}
body := request.body()
result := db.update("` + usersTable + `", me.user_id, body)
response.json(result)`,
		},
		{
			Name: "auth_forgot_password",
			Code: `body := request.body()
if body.email == undefined {
  response.fail(400, "email required")
}
user := db.query_one("SELECT id FROM ` + usersTable + ` WHERE email = ? AND deleted_at IS NULL", [body.email])
if user == undefined {
  response.fail(404, "email not found")
}
token := crypto.uuid()
db.insert("password_resets", {user_id: user.id, token: token, expires_at: date.add_days(date.now(), 1)})
response.json({reset_token: token, message: "use this token to reset your password"})`,
		},
		{
			Name: "auth_reset_password",
			Code: `body := request.body()
if body.token == undefined || body.password == undefined {
  response.fail(400, "token and password required")
}
reset := db.query_one("SELECT * FROM password_resets WHERE token = ? AND used = 0", [body.token])
if reset == undefined {
  response.fail(400, "invalid or expired reset token")
}
hashed := crypto.hash_password(body.password)
db.query("UPDATE ` + usersTable + ` SET password_hash = ?, updated_at = date.now() WHERE id = ?", [hashed, reset.user_id])
db.query("UPDATE password_resets SET used = 1 WHERE id = ?", [reset.id])
response.json({message: "password reset successful"})`,
		},
		{
			Name: "auth_verify_email",
			Code: `me := request.auth()
if me == undefined {
  response.fail(401, "authentication required")
}
db.query("UPDATE ` + usersTable + ` SET email_verified = 1, updated_at = date.now() WHERE id = ?", [me.user_id])
response.json({message: "email verified"})`,
		},
	}

	return routes, scripts
}
