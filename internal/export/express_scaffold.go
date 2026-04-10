package export

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GeneratePackageJSON creates a package.json for the Express project.
func GeneratePackageJSON(m *manifest.Manifest, dbType string) string {
	type PackageJSON struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Description     string            `json:"description"`
		Main            string            `json:"main"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		Engines         map[string]string `json:"engines"`
	}

	deps := map[string]string{
		"express":            "^4.21.0",
		"helmet":             "^8.0.0",
		"cors":               "^2.8.5",
		"express-rate-limit": "^7.4.0",
		"jsonwebtoken":       "^9.0.2",
		"bcryptjs":           "^2.4.3",
		"express-validator":  "^7.2.0",
		"morgan":             "^1.10.0",
		"dotenv":             "^16.4.5",
		"compression":        "^1.7.4",
	}
	if dbType == "postgres" {
		deps["pg"] = "^8.13.0"
	} else {
		deps["better-sqlite3"] = "^11.6.0"
	}

	pkg := PackageJSON{
		Name:        Slugify(m.Name),
		Version:     m.Version,
		Description: m.Description,
		Main:        "server.js",
		Scripts: map[string]string{
			"start":     "node server.js",
			"dev":       "node --watch server.js",
			"seed":      "node src/models/database.js",
		},
		Dependencies: deps,
		Engines: map[string]string{
			"node": ">=18.0.0",
		},
	}

	data, _ := json.MarshalIndent(pkg, "", "  ")
	return string(data) + "\n"
}

// GenerateServerJS creates the main server.js with all security middleware.
func GenerateServerJS(m *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString(`const dotenv = require('dotenv');
dotenv.config();

// ─── Environment Validation ───────────────────────────────────
const required = ['JWT_SECRET'];
for (const key of required) {
  if (!process.env[key]) {
    console.error('FATAL: Missing required environment variable: ' + key);
    console.error('Copy .env.example to .env and fill in the values.');
    process.exit(1);
  }
}

const express = require('express');
const helmet = require('helmet');
const cors = require('cors');
const rateLimit = require('express-rate-limit');
const morgan = require('morgan');
const compression = require('compression');

const routes = require('./src/routes');
const { initDB } = require('./src/models/database');

const app = express();
const PORT = process.env.PORT || 3000;

// ─── Security Middleware ────────────────────────────────────────

// Security headers
app.use(helmet());

// CORS with configurable origin
app.use(cors({
  origin: process.env.CORS_ORIGIN || 'http://localhost:3000',
  methods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'],
  allowedHeaders: ['Content-Type', 'Authorization'],
  credentials: true,
}));

// General rate limiter: 100 requests per 15 minutes
const generalLimiter = rateLimit({
  windowMs: 15 * 60 * 1000,
  max: 100,
  standardHeaders: true,
  legacyHeaders: false,
  message: { error: 'Too many requests, please try again later.' },
});
app.use(generalLimiter);

// Stricter rate limiter for auth routes: 5 requests per 15 minutes
const authLimiter = rateLimit({
  windowMs: 15 * 60 * 1000,
  max: 5,
  standardHeaders: true,
  legacyHeaders: false,
  message: { error: 'Too many authentication attempts.' },
});
`)

	// Apply auth limiter to auth-related routes if they exist
	for _, route := range m.Routes {
		if strings.Contains(route.Path, "auth") || strings.Contains(route.Path, "login") || strings.Contains(route.Path, "register") {
			method := strings.ToLower(route.Method)
			b.WriteString(fmt.Sprintf("app.%s('%s', authLimiter);\n", method, expressPath(route.Path)))
		}
	}

	b.WriteString(`
// ─── Parsing & Compression ─────────────────────────────────────

app.use(compression());
app.use(express.json({ limit: '1mb' }));

// Request logging
app.use(morgan('combined'));

// ─── Health Check ───────────────────────────────────────────────

app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// ─── API Routes ─────────────────────────────────────────────────

app.use('/', routes);

// ─── 404 Handler ────────────────────────────────────────────────

app.use((req, res) => {
  res.status(404).json({ error: 'Not found' });
});

// ─── Error Handling Middleware ───────────────────────────────────

app.use((err, req, res, _next) => {
  console.error('Unhandled error:', err);
  const status = err.status || 500;
  const message = process.env.NODE_ENV === 'production'
    ? 'Internal server error'
    : err.message;
  res.status(status).json({ error: message });
});

// ─── Start Server ───────────────────────────────────────────────

async function start() {
  try {
    // Initialize database
    await initDB();
    console.log('Database initialized');

    const server = app.listen(PORT, () => {
      console.log(` + "`" + `Server running on port ${PORT}` + "`" + `);
    });

    // Graceful shutdown
    const shutdown = (signal) => {
      console.log(` + "`" + `\n${signal} received, shutting down gracefully...` + "`" + `);
      server.close(async () => {
        try {
          const { closeDB } = require('./src/models/database');
          if (closeDB) await closeDB();
        } catch {}
        console.log('Server closed');
        process.exit(0);
      });

      // Force close after 30 seconds
      setTimeout(() => {
        console.error('Forced shutdown after timeout');
        process.exit(1);
      }, 30000);
    };

    process.on('SIGINT', () => shutdown('SIGINT'));
    process.on('SIGTERM', () => shutdown('SIGTERM'));
  } catch (err) {
    console.error('Failed to start server:', err);
    process.exit(1);
  }
}

start();
`)

	return b.String()
}

// GenerateEnvExample creates the .env.example file.
func GenerateEnvExample(dbType string) string {
	var b strings.Builder
	b.WriteString("# Server\nPORT=3000\nNODE_ENV=development\n\n")
	if dbType == "postgres" {
		b.WriteString("# Database\nDATABASE_URL=postgresql://user:password@localhost:5432/mydb\n\n")
	} else {
		b.WriteString("# Database\nDATABASE_PATH=./src/data/state.db\n\n")
	}
	b.WriteString("# Security\nJWT_SECRET=\nJWT_EXPIRES_IN=24h\n\n# CORS\nCORS_ORIGIN=http://localhost:3000\n")
	return b.String()
}

// GenerateExpressDockerfile creates a Node.js Dockerfile.
func GenerateExpressDockerfile(dbType string) string {
	var b strings.Builder
	b.WriteString(`FROM node:20-alpine AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=production

FROM node:20-alpine

WORKDIR /app

COPY --from=builder /app/node_modules ./node_modules
COPY . .

`)
	if dbType == "sqlite" {
		b.WriteString("RUN mkdir -p src/data\n\n")
	}
	b.WriteString(`EXPOSE 3000

ENV NODE_ENV=production
ENV PORT=3000
`)
	if dbType == "sqlite" {
		b.WriteString("ENV DATABASE_PATH=./src/data/state.db\n")
	}
	b.WriteString(`
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/health', (r) => {process.exit(r.statusCode === 200 ? 0 : 1)})"

USER node

CMD ["node", "server.js"]
`)
	return b.String()
}

// GenerateExpressREADME creates a README for the Express project.
func GenerateExpressREADME(m *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", m.Name))
	if m.Description != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", m.Description))
	}

	b.WriteString("## Quick Start\n\n")
	b.WriteString("```bash\n")
	b.WriteString("cp .env.example .env\n")
	b.WriteString("npm install\n")
	b.WriteString("npm start\n")
	b.WriteString("```\n\n")

	b.WriteString("## Development\n\n")
	b.WriteString("```bash\n")
	b.WriteString("npm run dev\n")
	b.WriteString("```\n\n")

	b.WriteString("## Docker\n\n")
	b.WriteString("```bash\n")
	b.WriteString("docker build -t " + Slugify(m.Name) + " .\n")
	b.WriteString("docker run -p 3000:3000 " + Slugify(m.Name) + "\n")
	b.WriteString("```\n\n")

	b.WriteString("## Security Features\n\n")
	b.WriteString("- **Helmet** - Security HTTP headers\n")
	b.WriteString("- **CORS** - Configurable cross-origin resource sharing\n")
	b.WriteString("- **Rate Limiting** - 100 req/15min general, 5 req/15min auth routes\n")
	b.WriteString("- **JWT Authentication** - Token-based auth via `Authorization: Bearer <token>`\n")
	b.WriteString("- **bcrypt** - Password hashing\n")
	b.WriteString("- **express-validator** - Input validation on all POST/PUT/PATCH routes\n")
	b.WriteString("- **Compression** - gzip response compression\n")
	b.WriteString("- **Morgan** - HTTP request logging\n")
	b.WriteString("- **Request Size Limit** - 1MB JSON body limit\n\n")

	if len(m.Routes) > 0 {
		b.WriteString("## Routes\n\n")
		b.WriteString("| Method | Path | Description |\n")
		b.WriteString("|--------|------|-------------|\n")
		for _, route := range m.Routes {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", route.Method, route.Path, route.Description))
		}
		b.WriteString("\n")
	}

	if len(m.Schemas) > 0 {
		b.WriteString("## Schema\n\n")
		for _, schema := range m.Schemas {
			b.WriteString(fmt.Sprintf("### %s\n\n", schema.Table))
			b.WriteString("| Column | Type | Constraints |\n")
			b.WriteString("|--------|------|-------------|\n")
			for _, col := range schema.Columns {
				constraints := buildConstraints(col)
				b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", col.Name, col.Type, constraints))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Environment Variables\n\n")
	b.WriteString("| Variable | Default | Description |\n")
	b.WriteString("|----------|---------|-------------|\n")
	b.WriteString("| PORT | 3000 | HTTP server port |\n")
	b.WriteString("| NODE_ENV | development | Node environment |\n")
	b.WriteString("| DATABASE_PATH | ./src/data/state.db | SQLite database path |\n")
	b.WriteString("| JWT_SECRET | (required) | Secret key for JWT signing |\n")
	b.WriteString("| JWT_EXPIRES_IN | 24h | JWT token expiry |\n")
	b.WriteString("| CORS_ORIGIN | * | Allowed CORS origin |\n")

	return b.String()
}

// GenerateGitignore creates a .gitignore for the Express project.
func GenerateGitignore() string {
	return `node_modules/
.env
*.db
dist/
coverage/
.DS_Store
`
}

// expressPath converts :id style params to Express-compatible format.
func expressPath(path string) string {
	return path // Express uses :id natively, no conversion needed
}
