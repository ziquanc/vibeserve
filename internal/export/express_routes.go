package export

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateAuthMiddleware creates src/middleware/auth.js with JWT verification.
func GenerateAuthMiddleware() string {
	return `const jwt = require('jsonwebtoken');

const JWT_SECRET = process.env.JWT_SECRET || 'change-me-to-a-random-secret';

/**
 * JWT authentication middleware.
 * Verifies the Bearer token from the Authorization header.
 *
 * Usage:
 *   const { authenticate, optionalAuth } = require('../middleware/auth');
 *   app.get('/protected', authenticate, handler);
 *   app.get('/optional', optionalAuth, handler);
 */
function authenticate(req, res, next) {
  const authHeader = req.headers.authorization;

  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'Access denied. No token provided.' });
  }

  const token = authHeader.split(' ')[1];

  try {
    const decoded = jwt.verify(token, JWT_SECRET);
    req.user = decoded;
    next();
  } catch (err) {
    if (err.name === 'TokenExpiredError') {
      return res.status(401).json({ error: 'Token expired.' });
    }
    return res.status(403).json({ error: 'Invalid token.' });
  }
}

/**
 * Optional auth middleware — sets req.user if token is present
 * but does not reject the request if missing.
 */
function optionalAuth(req, res, next) {
  const authHeader = req.headers.authorization;

  if (authHeader && authHeader.startsWith('Bearer ')) {
    const token = authHeader.split(' ')[1];
    try {
      req.user = jwt.verify(token, JWT_SECRET);
    } catch {
      // Ignore invalid tokens for optional auth
    }
  }
  next();
}

/**
 * Generate a JWT token for a user payload.
 */
function generateToken(payload, expiresIn) {
  return jwt.sign(payload, JWT_SECRET, {
    expiresIn: expiresIn || process.env.JWT_EXPIRES_IN || '24h',
  });
}

/**
 * Hash a password using bcryptjs.
 */
async function hashPassword(password) {
  const bcrypt = require('bcryptjs');
  const salt = await bcrypt.genSalt(10);
  return bcrypt.hash(password, salt);
}

/**
 * Compare a plain-text password against a bcrypt hash.
 */
async function comparePassword(password, hash) {
  const bcrypt = require('bcryptjs');
  return bcrypt.compare(password, hash);
}

module.exports = {
  authenticate,
  optionalAuth,
  generateToken,
  hashPassword,
  comparePassword,
};
`
}

// GenerateValidateMiddleware creates src/middleware/validate.js with
// express-validator helper utilities.
func GenerateValidateMiddleware() string {
	return `const { validationResult } = require('express-validator');

/**
 * Middleware that checks express-validator results and returns
 * a 400 with the validation errors if any rules failed.
 *
 * Usage:
 *   const { validate, body, query, param } = require('../middleware/validate');
 *   app.post('/users', validate([
 *     body('name').trim().notEmpty().withMessage('Name is required'),
 *     body('email').isEmail().withMessage('Valid email required'),
 *   ]), handler);
 */
function validate(rules) {
  return [
    ...rules,
    (req, res, next) => {
      const errors = validationResult(req);
      if (!errors.isEmpty()) {
        return res.status(400).json({
          error: 'Validation failed',
          details: errors.array().map(e => ({
            field: e.path,
            message: e.msg,
            value: e.value,
          })),
        });
      }
      next();
    },
  ];
}

/**
 * Re-export express-validator functions for convenience.
 */
const { body, param, query, header } = require('express-validator');

module.exports = {
  validate,
  body,
  param,
  query,
  header,
};
`
}

// --- Route generation ---

// expressRouteEntry describes a generated route file.
type expressRouteEntry struct {
	filename string
	resource string
}

// GenerateExpressRoutes generates all route files and returns a map of
// filename -> file content. It always includes an index.js that aggregates
// all per-resource route files.
func GenerateExpressRoutes(schemas []manifest.Schema, routes []manifest.Route, scripts []manifest.Script) map[string]string {
	// Build script lookup
	scriptMap := make(map[string]manifest.Script, len(scripts))
	for _, s := range scripts {
		scriptMap[s.Name] = s
	}

	// Group routes by resource (derived from path)
	resourceRoutes := make(map[string][]manifest.Route)
	var resourceOrder []string

	for _, route := range routes {
		resource := inferExpressResource(route.Path)
		if _, exists := resourceRoutes[resource]; !exists {
			resourceOrder = append(resourceOrder, resource)
		}
		resourceRoutes[resource] = append(resourceRoutes[resource], route)
	}

	files := make(map[string]string)

	// Generate per-resource route files
	for _, resource := range resourceOrder {
		routesForResource := resourceRoutes[resource]
		filename := resource + ".js"
		files[filename] = generateResourceRoutes(schemas, resource, routesForResource, scriptMap)
	}

	// Generate routes/index.js
	files["index.js"] = generateRouteIndex(resourceOrder)

	return files
}

// generateResourceRoutes creates the Express router file for one resource.
func generateResourceRoutes(schemas []manifest.Schema, resource string, routes []manifest.Route, scriptMap map[string]manifest.Script) string {
	var b strings.Builder

	b.WriteString("const express = require('express');\n")
	b.WriteString("const router = express.Router();\n")

	// Determine which imports we need
	needsDB := false
	needsValidator := false
	for _, route := range routes {
		script, ok := scriptMap[route.Script]
		if ok {
			code := script.Code
			if strings.Contains(code, "db.") {
				needsDB = true
			}
			if strings.Contains(code, "request.body()") || len(route.RequestBody) > 0 {
				needsValidator = true
			}
		}
		if len(route.RequestBody) > 0 {
			needsValidator = true
		}
	}

	b.WriteString("\n")

	// Determine the struct name for the resource for DB imports
	structName := TableToStructName(resource)
	pluralName := structName + "s"
	// Find the schema for this resource if it exists
	schemaMap := make(map[string]manifest.Schema, len(schemas))
	for _, s := range schemas {
		schemaMap[s.Table] = s
	}

	if needsDB {
		b.WriteString("const { getDB")
		// Import specific helper functions
		_, hasSchema := schemaMap[resource]
		if hasSchema {
			b.WriteString(fmt.Sprintf(", list%s, get%s, create%s, update%s, delete%s",
				pluralName, structName, structName, structName, structName))
		}
		b.WriteString(" } = require('../models/database');\n")
	}

	if needsValidator {
		b.WriteString("const { validate, body, param, query: queryValidator } = require('../middleware/validate');\n")
	}

	b.WriteString("\n")

	// Generate each route handler
	for _, route := range routes {
		method := strings.ToLower(route.Method)
		expressPathStr := expressPath(route.Path)
		handlerName := routeToMethodName(route)

		b.WriteString(fmt.Sprintf("// %s: %s %s\n", handlerName, route.Method, route.Path))
		if route.Description != "" {
			b.WriteString(fmt.Sprintf("// %s\n", route.Description))
		}

		// Build the route registration line with optional middleware
		var middleware []string
		if len(route.RequestBody) > 0 {
			validationRules := generateValidationRules(route.RequestBody)
			middleware = append(middleware, fmt.Sprintf("validate([%s])", validationRules))
		}

		// Path parameter validation for routes with :id etc.
		pathParams := extractPathParams(route.Path)
		for _, p := range pathParams {
			colType := findColumnType(schemas, p)
			switch colType {
			case "INTEGER":
				middleware = append(middleware, fmt.Sprintf("param('%s').isInt().withMessage('%s must be an integer')", p, p))
			default:
				middleware = append(middleware, fmt.Sprintf("param('%s').notEmpty().withMessage('%s is required')", p, p))
			}
		}

		middlewareStr := ""
		if len(middleware) > 0 {
			middlewareStr = ", " + strings.Join(middleware, ", ")
		}

		b.WriteString(fmt.Sprintf("router.%s('%s'%s, (req, res, next) => {\n", method, expressPathStr, middlewareStr))
		b.WriteString("  try {\n")

		// Generate handler body
		script, hasScript := scriptMap[route.Script]
		if hasScript {
			jsBody := translateTengoToJS(script.Code, route, schemas)
			if jsBody == "" {
				// Fallback stub
				b.WriteString(fmt.Sprintf("    // TODO: Implement %s\n", route.Script))
				b.WriteString("    res.status(501).json({ error: 'Not implemented' });\n")
			} else {
				for _, line := range strings.Split(jsBody, "\n") {
					if strings.TrimSpace(line) == "" {
						b.WriteString("\n")
					} else {
						b.WriteString("  " + line + "\n")
					}
				}
			}
		} else {
			b.WriteString("    res.status(501).json({ error: 'No script defined' });\n")
		}

		b.WriteString("  } catch (err) {\n")
		b.WriteString("    next(err);\n")
		b.WriteString("  }\n")
		b.WriteString("});\n\n")
	}

	b.WriteString("module.exports = router;\n")

	return b.String()
}

// generateRouteIndex creates routes/index.js that mounts all resource routers.
func generateRouteIndex(resources []string) string {
	var b strings.Builder

	b.WriteString("const express = require('express');\n")
	b.WriteString("const router = express.Router();\n\n")

	for _, resource := range resources {
		b.WriteString(fmt.Sprintf("const %sRoutes = require('./%s');\n", resource, resource))
	}

	b.WriteString("\n")

	// Mount routes — we try to figure out the base path from the resource name
	for _, resource := range resources {
		b.WriteString(fmt.Sprintf("router.use('/%s', %sRoutes);\n", resource, resource))
	}

	b.WriteString("\nmodule.exports = router;\n")

	return b.String()
}

// generateValidationRules creates express-validator rules from a RequestBody map.
func generateValidationRules(requestBody map[string]string) string {
	var rules []string
	for field, colType := range requestBody {
		rule := generateFieldValidation(field, colType)
		rules = append(rules, rule)
	}
	return strings.Join(rules, ", ")
}

// generateFieldValidation creates a single field validation chain.
func generateFieldValidation(field, colType string) string {
	switch strings.ToUpper(colType) {
	case "INTEGER", "INT":
		return fmt.Sprintf("body('%s').isInt().withMessage('%s must be an integer')", field, field)
	case "TEXT", "VARCHAR", "STRING":
		return fmt.Sprintf("body('%s').isString().trim().notEmpty().withMessage('%s is required')", field, field)
	case "REAL", "FLOAT", "DOUBLE":
		return fmt.Sprintf("body('%s').isFloat().withMessage('%s must be a number')", field, field)
	case "BOOLEAN", "BOOL":
		return fmt.Sprintf("body('%s').isBoolean().withMessage('%s must be a boolean')", field, field)
	case "EMAIL":
		return fmt.Sprintf("body('%s').isEmail().withMessage('%s must be a valid email')", field, field)
	default:
		return fmt.Sprintf("body('%s').notEmpty().withMessage('%s is required')", field, field)
	}
}

// --- Tengo to JS translation ---

var (
	reJSRequestParam = regexp.MustCompile(`^(\w+)\s*:=\s*request\.param\("(\w+)"\)`)
	reJSRequestQuery = regexp.MustCompile(`^(\w+)\s*:=\s*request\.query\("(\w+)"\)`)
	reJSRequestBody  = regexp.MustCompile(`^(\w+)\s*:=\s*request\.body\(\)`)
	reJSDBQuery      = regexp.MustCompile(`^(\w+)\s*:=\s*db\.query\("([^"]+)",\s*\[([^\]]*)\]\)`)
	reJSDBQueryOne   = regexp.MustCompile(`^(\w+)\s*:=\s*db\.query_one\("([^"]+)",\s*\[([^\]]*)\]\)`)
	reJSDBInsert     = regexp.MustCompile(`^(\w+)\s*:=\s*db\.insert\("(\w+)",\s*(.+)`)
	reJSDBInsertStmt = regexp.MustCompile(`^db\.insert\("(\w+)",\s*(.+)`)
	reJSDBUpdate     = regexp.MustCompile(`^(\w+)\s*:=\s*db\.update\("(\w+)",\s*(\w+),\s*(.+)`)
	reJSDBUpdateStmt = regexp.MustCompile(`^db\.update\("(\w+)",\s*(\w+),\s*(.+)`)
	reJSDBDelete     = regexp.MustCompile(`^(\w+)\s*:=\s*db\.delete\("(\w+)",\s*(\w+)`)
	reJSDBDeleteStmt = regexp.MustCompile(`^db\.delete\("(\w+)",\s*(\w+)`)
	reJSResponseJSON = regexp.MustCompile(`^response\.json\((\w+)(?:,\s*(\d+))?\)`)
	reJSResponseFail = regexp.MustCompile(`^response\.fail\((\d+),\s*"([^"]*)"\)`)
	reJSIfUndefined  = regexp.MustCompile(`^if\s+(\w+)\s*==\s*undefined\s*\{`)
	reJSElse         = regexp.MustCompile(`^\}\s*else\s*\{`)
	reJSCloseBrace   = regexp.MustCompile(`^\}$`)
)

// translateTengoToJS converts Tengo script code to JavaScript handler code.
// For complex scripts that can't be translated, it adds TODO comments.
func translateTengoToJS(code string, route manifest.Route, schemas []manifest.Schema) string {
	lines := strings.Split(code, "\n")
	var out []string

	// Build schema lookup
	schemaMap := make(map[string]manifest.Schema, len(schemas))
	for _, s := range schemas {
		schemaMap[s.Table] = s
	}

	// Track state
	bodyVar := ""

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		indent := leadingWhitespace(line)

		if trimmed == "" {
			out = append(out, "")
			continue
		}

		// Close brace
		if reJSCloseBrace.MatchString(trimmed) {
			out = append(out, indent+"}")
			continue
		}

		// else {
		if reJSElse.MatchString(trimmed) {
			out = append(out, indent+"} else {")
			continue
		}

		// if x == undefined
		if m := reJSIfUndefined.FindStringSubmatch(trimmed); m != nil {
			out = append(out, indent+fmt.Sprintf("if (%s === undefined) {", m[1]))
			continue
		}

		// request.param("x")
		if m := reJSRequestParam.FindStringSubmatch(trimmed); m != nil {
			out = append(out, indent+fmt.Sprintf("const %s = req.params.%s;", m[1], m[2]))
			continue
		}

		// request.query("x")
		if m := reJSRequestQuery.FindStringSubmatch(trimmed); m != nil {
			out = append(out, indent+fmt.Sprintf("const %s = req.query.%s;", m[1], m[2]))
			continue
		}

		// request.body()
		if m := reJSRequestBody.FindStringSubmatch(trimmed); m != nil {
			bodyVar = m[1]
			out = append(out, indent+fmt.Sprintf("const %s = req.body;", bodyVar))
			continue
		}

		// db.query("SELECT ...", [params])
		if m := reJSDBQuery.FindStringSubmatch(trimmed); m != nil {
			varName, sql, params := m[1], m[2], m[3]
			jsParams := jsTranslateParams(params)
			out = append(out, indent+fmt.Sprintf("const database = getDB();"))
			if jsParams != "" {
				out = append(out, indent+fmt.Sprintf("const %s = database.prepare('%s').all(%s);", varName, escapeSQLForJS(sql), jsParams))
			} else {
				out = append(out, indent+fmt.Sprintf("const %s = database.prepare('%s').all();", varName, escapeSQLForJS(sql)))
			}
			continue
		}

		// db.query_one("SELECT ...", [params])
		if m := reJSDBQueryOne.FindStringSubmatch(trimmed); m != nil {
			varName, sql, params := m[1], m[2], m[3]
			jsParams := jsTranslateParams(params)
			out = append(out, indent+fmt.Sprintf("const database = getDB();"))
			if jsParams != "" {
				out = append(out, indent+fmt.Sprintf("const %s = database.prepare('%s').get(%s);", varName, escapeSQLForJS(sql), jsParams))
			} else {
				out = append(out, indent+fmt.Sprintf("const %s = database.prepare('%s').get();", varName, escapeSQLForJS(sql)))
			}
			continue
		}

		// db.insert("table", data)
		if m := reJSDBInsert.FindStringSubmatch(trimmed); m != nil {
			varName, table := m[1], m[2]
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "req.body"
			}
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("const %s = create%s(%s);", varName, structName, dataVar))
			continue
		}
		if m := reJSDBInsertStmt.FindStringSubmatch(trimmed); m != nil {
			table := m[1]
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "req.body"
			}
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("create%s(%s);", structName, dataVar))
			continue
		}

		// db.update("table", id, data)
		if m := reJSDBUpdate.FindStringSubmatch(trimmed); m != nil {
			varName, table, idVar := m[1], m[2], m[3]
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "req.body"
			}
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("const %s = update%s(%s, %s);", varName, structName, idVar, dataVar))
			continue
		}
		if m := reJSDBUpdateStmt.FindStringSubmatch(trimmed); m != nil {
			table, idVar := m[1], m[2]
			dataVar := bodyVar
			if dataVar == "" {
				dataVar = "req.body"
			}
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("update%s(%s, %s);", structName, idVar, dataVar))
			continue
		}

		// db.delete("table", id)
		if m := reJSDBDelete.FindStringSubmatch(trimmed); m != nil {
			varName, table, idVar := m[1], m[2], m[3]
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("delete%s(%s);", structName, idVar))
			_ = varName
			continue
		}
		if m := reJSDBDeleteStmt.FindStringSubmatch(trimmed); m != nil {
			table, idVar := m[1], m[2]
			structName := TableToStructName(table)
			out = append(out, indent+fmt.Sprintf("delete%s(%s);", structName, idVar))
			continue
		}

		// response.json(data) or response.json(data, status)
		if m := reJSResponseJSON.FindStringSubmatch(trimmed); m != nil {
			dataVar, statusStr := m[1], m[2]
			if statusStr != "" {
				out = append(out, indent+fmt.Sprintf("res.status(%s).json(%s);", statusStr, dataVar))
			} else {
				out = append(out, indent+fmt.Sprintf("res.json(%s);", dataVar))
			}
			continue
		}

		// response.fail(status, "msg")
		if m := reJSResponseFail.FindStringSubmatch(trimmed); m != nil {
			status, msg := m[1], m[2]
			out = append(out, indent+fmt.Sprintf("return res.status(%s).json({ error: '%s' });", status, msg))
			continue
		}

		// Unrecognized: emit as TODO comment
		out = append(out, indent+fmt.Sprintf("// TODO: Manual translation needed: %s", trimmed))
	}

	return strings.Join(out, "\n")
}

// jsTranslateParams converts Tengo param list "id, true" to JS "id, true".
func jsTranslateParams(params string) string {
	params = strings.TrimSpace(params)
	if params == "" {
		return ""
	}
	parts := strings.Split(params, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ", ")
}

// escapeSQLForJS escapes single quotes in SQL strings for JS single-quoted strings.
func escapeSQLForJS(sql string) string {
	return strings.ReplaceAll(sql, "'", "''")
}

// inferExpressResource extracts the resource name from a route path.
// e.g. /vehicles/:id → "vehicles", /bookings → "bookings"
func inferExpressResource(path string) string {
	segments := strings.Split(strings.TrimRight(path, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if seg == "" || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "{") {
			continue
		}
		return seg
	}
	return "api"
}

// findColumnType looks up the column type for a given parameter name across all schemas.
func findColumnType(schemas []manifest.Schema, paramName string) string {
	for _, s := range schemas {
		for _, c := range s.Columns {
			if c.Name == paramName {
				return c.Type
			}
		}
	}
	return ""
}

// leadingWhitespace returns the leading whitespace of a line.
func leadingWhitespace(line string) string {
	var sb strings.Builder
	for _, ch := range line {
		if ch == '\t' || ch == ' ' {
			sb.WriteRune(ch)
		} else {
			break
		}
	}
	return sb.String()
}
