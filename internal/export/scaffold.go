package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateMain produces package main source for cmd/api/main.go.
// It registers all routes using chi router with standard middleware and
// graceful shutdown support.
func GenerateMain(moduleName string, routes []manifest.Route) string {
	var b strings.Builder

	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"log\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"os/signal\"\n")
	b.WriteString("\t\"syscall\"\n")
	b.WriteString("\t\"time\"\n\n")
	b.WriteString(fmt.Sprintf("\t\"%s/internal/handler\"\n", moduleName))
	b.WriteString(fmt.Sprintf("\t\"%s/internal/repository\"\n\n", moduleName))
	b.WriteString("\t\"github.com/go-chi/chi/v5\"\n")
	b.WriteString("\t\"github.com/go-chi/chi/v5/middleware\"\n")
	b.WriteString(")\n\n")

	b.WriteString("func main() {\n")
	b.WriteString("\tdsn := os.Getenv(\"DATABASE_URL\")\n")
	b.WriteString("\tif dsn == \"\" {\n")
	b.WriteString("\t\tdsn = \"state.db\"\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\tstore, err := repository.NewSQLiteStore(dsn)\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"open db: %v\", err)\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\th := handler.NewHandler(store)\n\n")
	b.WriteString("\tr := chi.NewRouter()\n")
	b.WriteString("\tr.Use(middleware.RequestID)\n")
	b.WriteString("\tr.Use(middleware.RealIP)\n")
	b.WriteString("\tr.Use(middleware.Logger)\n")
	b.WriteString("\tr.Use(middleware.Recoverer)\n\n")

	for _, route := range routes {
		method := strings.ToUpper(route.Method[:1]) + strings.ToLower(route.Method[1:])
		path := chiPath(route.Path)
		handlerMethod := routeToMethodName(route)
		b.WriteString(fmt.Sprintf("\tr.%s(%q, h.%s)\n", method, path, handlerMethod))
	}

	b.WriteString("\n")
	b.WriteString("\tport := os.Getenv(\"PORT\")\n")
	b.WriteString("\tif port == \"\" {\n")
	b.WriteString("\t\tport = \"8080\"\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\tsrv := &http.Server{\n")
	b.WriteString("\t\tAddr:    fmt.Sprintf(\":%s\", port),\n")
	b.WriteString("\t\tHandler: r,\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\tgo func() {\n")
	b.WriteString("\t\tlog.Printf(\"listening on :%s\", port)\n")
	b.WriteString("\t\tif err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {\n")
	b.WriteString("\t\t\tlog.Fatalf(\"listen: %v\", err)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}()\n\n")
	b.WriteString("\tquit := make(chan os.Signal, 1)\n")
	b.WriteString("\tsignal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)\n")
	b.WriteString("\t<-quit\n\n")
	b.WriteString("\tctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)\n")
	b.WriteString("\tdefer cancel()\n")
	b.WriteString("\tif err := srv.Shutdown(ctx); err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"shutdown: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")

	return b.String()
}

// GenerateGoMod produces a go.mod file for the exported project.
// It includes chi v5, sqlx, and modernc.org/sqlite as dependencies.
func GenerateGoMod(moduleName string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("module %s\n\n", moduleName))
	b.WriteString("go 1.23\n\n")
	b.WriteString("require (\n")
	b.WriteString("\tgithub.com/go-chi/chi/v5 v5.1.0\n")
	b.WriteString("\tgithub.com/jmoiron/sqlx v1.4.0\n")
	b.WriteString("\tmodernc.org/sqlite v1.33.1\n")
	b.WriteString(")\n")

	return b.String()
}

// GenerateDockerfile produces a multi-stage Dockerfile.
// Stage 1: golang:1.23-alpine builder with CGO_ENABLED=0.
// Stage 2: alpine runtime with binary and EXPOSE 8080.
func GenerateDockerfile(moduleName string) string {
	var b strings.Builder

	b.WriteString("FROM golang:1.23-alpine AS builder\n\n")
	b.WriteString("WORKDIR /app\n\n")
	b.WriteString("COPY go.mod go.sum ./\n")
	b.WriteString("RUN go mod download\n\n")
	b.WriteString("COPY . .\n\n")
	b.WriteString(fmt.Sprintf("RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api ./cmd/api\n\n"))
	b.WriteString("FROM alpine:3.20\n\n")
	b.WriteString("WORKDIR /app\n\n")
	b.WriteString("COPY --from=builder /app/api /app/api\n")
	b.WriteString("COPY --from=builder /app/state.db* /app/\n\n")
	b.WriteString("EXPOSE 8080\n\n")
	b.WriteString("ENV DATABASE_URL=state.db\n")
	b.WriteString("ENV PORT=8080\n\n")
	b.WriteString("CMD [\"/app/api\"]\n")

	return b.String()
}

// GenerateREADME produces a markdown README for the exported project.
// It includes project name, description, quick start, Docker instructions,
// route table, schema documentation, and environment variables.
func GenerateREADME(m *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", m.Name))
	if m.Description != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", m.Description))
	}

	b.WriteString("## Quick Start\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go run ./cmd/api\n")
	b.WriteString("```\n\n")

	b.WriteString("## Docker\n\n")
	b.WriteString("```bash\n")
	b.WriteString("docker build -t api .\n")
	b.WriteString("docker run -p 8080:8080 api\n")
	b.WriteString("```\n\n")

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
	b.WriteString("| DATABASE_URL | state.db | SQLite database path |\n")
	b.WriteString("| PORT | 8080 | HTTP server port |\n")

	return b.String()
}

// buildConstraints formats column constraints as a comma-separated string.
func buildConstraints(col manifest.Column) string {
	var parts []string
	if col.Primary {
		parts = append(parts, "PRIMARY KEY")
	}
	if col.Auto {
		parts = append(parts, "AUTOINCREMENT")
	}
	if col.Required {
		parts = append(parts, "NOT NULL")
	}
	if col.Unique {
		parts = append(parts, "UNIQUE")
	}
	if col.References != "" {
		parts = append(parts, fmt.Sprintf("REFERENCES %s", col.References))
	}
	return strings.Join(parts, ", ")
}
