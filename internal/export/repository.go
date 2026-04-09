package export

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Regex patterns for detecting database operations in script code.
var (
	reDBQuery    = regexp.MustCompile(`db\.query\(.*FROM\s+(\w+)`)
	reDBQueryOne = regexp.MustCompile(`db\.query_one\(.*FROM\s+(\w+)`)
	reDBInsert   = regexp.MustCompile(`db\.insert\("(\w+)"`)
	reDBUpdate   = regexp.MustCompile(`db\.update\("(\w+)"`)
	reDBDelete   = regexp.MustCompile(`db\.delete\("(\w+)"`)
)

// RouteUsage tracks which CRUD methods are used per table.
type RouteUsage struct {
	methods map[string]map[string]bool
}

// HasMethod reports whether a given method (List/Get/Create/Update/Delete) is
// used for the given table.
func (u *RouteUsage) HasMethod(table, method string) bool {
	if u.methods == nil {
		return false
	}
	return u.methods[table][method]
}

// Tables returns the sorted list of tables that appear in the usage map.
func (u *RouteUsage) Tables() []string {
	tables := make([]string, 0, len(u.methods))
	for t := range u.methods {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	return tables
}

// Methods returns the sorted list of methods used for a given table.
func (u *RouteUsage) Methods(table string) []string {
	order := []string{"List", "Get", "Create", "Update", "Delete"}
	var out []string
	for _, m := range order {
		if u.methods[table][m] {
			out = append(out, m)
		}
	}
	return out
}

func (u *RouteUsage) addMethod(table, method string) {
	if u.methods == nil {
		u.methods = make(map[string]map[string]bool)
	}
	if u.methods[table] == nil {
		u.methods[table] = make(map[string]bool)
	}
	u.methods[table][method] = true
}

// AnalyzeRouteUsage scans script code for db.* calls and returns a RouteUsage
// that maps tables to the CRUD methods that need to be generated.
func AnalyzeRouteUsage(routes []manifest.Route, scripts []manifest.Script) *RouteUsage {
	// Build script name -> code map for the routes we care about.
	scriptMap := make(map[string]string, len(scripts))
	for _, s := range scripts {
		scriptMap[s.Name] = s.Code
	}

	usage := &RouteUsage{}

	for _, r := range routes {
		code, ok := scriptMap[r.Script]
		if !ok {
			continue
		}

		// db.query → List
		for _, m := range reDBQuery.FindAllStringSubmatch(code, -1) {
			usage.addMethod(m[1], "List")
		}
		// db.query_one → Get
		for _, m := range reDBQueryOne.FindAllStringSubmatch(code, -1) {
			usage.addMethod(m[1], "Get")
		}
		// db.insert → Create
		for _, m := range reDBInsert.FindAllStringSubmatch(code, -1) {
			usage.addMethod(m[1], "Create")
		}
		// db.update → Update
		for _, m := range reDBUpdate.FindAllStringSubmatch(code, -1) {
			usage.addMethod(m[1], "Update")
		}
		// db.delete → Delete
		for _, m := range reDBDelete.FindAllStringSubmatch(code, -1) {
			usage.addMethod(m[1], "Delete")
		}
	}

	return usage
}

// GenerateStoreInterface produces the Go source for repository/store.go.
// It emits a Store interface whose methods are driven by RouteUsage.
// The string "{{MODULE}}" is a placeholder replaced by the pipeline orchestrator.
func GenerateStoreInterface(schemas []manifest.Schema, usage *RouteUsage) string {
	// Build a lookup from table name -> Schema for PK type resolution.
	schemaMap := make(map[string]manifest.Schema, len(schemas))
	for _, s := range schemas {
		schemaMap[s.Table] = s
	}

	var b strings.Builder

	b.WriteString("package repository\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n\n")
	b.WriteString("\t\"github.com/jmoiron/sqlx\"\n\n")
	b.WriteString("\t\"{{MODULE}}/internal/model\"\n")
	b.WriteString(")\n\n")

	b.WriteString("// Store is the data-access interface generated from the manifest.\n")
	b.WriteString("type Store interface {\n")

	for _, table := range usage.Tables() {
		schema, hasSchema := schemaMap[table]
		structName := TableToStructName(table)
		pluralName := structName + "s"

		pkType := "int64"
		if hasSchema {
			pkType = findPKType(schema)
		}

		for _, method := range usage.Methods(table) {
			switch method {
			case "List":
				b.WriteString(fmt.Sprintf("\tList%s(ctx context.Context) ([]model.%s, error)\n",
					pluralName, structName))
			case "Get":
				b.WriteString(fmt.Sprintf("\tGet%s(ctx context.Context, id %s) (*model.%s, error)\n",
					structName, pkType, structName))
			case "Create":
				b.WriteString(fmt.Sprintf("\tCreate%s(ctx context.Context, v *model.%s) error\n",
					structName, structName))
			case "Update":
				b.WriteString(fmt.Sprintf("\tUpdate%s(ctx context.Context, id %s, v *model.%s) error\n",
					structName, pkType, structName))
			case "Delete":
				b.WriteString(fmt.Sprintf("\tDelete%s(ctx context.Context, id %s) error\n",
					structName, pkType))
			}
		}
	}

	b.WriteString("\n")
	b.WriteString("\t// DB returns the underlying sqlx connection for advanced queries.\n")
	b.WriteString("\tDB() *sqlx.DB\n")
	b.WriteString("\tClose() error\n")
	b.WriteString("}\n")

	return b.String()
}

// GenerateSQLiteStore produces the Go source for repository/sqlite.go,
// providing a concrete SQLiteStore that implements Store.
func GenerateSQLiteStore(schemas []manifest.Schema, usage *RouteUsage) string {
	schemaMap := make(map[string]manifest.Schema, len(schemas))
	for _, s := range schemas {
		schemaMap[s.Table] = s
	}

	var b strings.Builder

	b.WriteString("package repository\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"fmt\"\n\n")
	b.WriteString("\t\"github.com/jmoiron/sqlx\"\n")
	b.WriteString("\t_ \"modernc.org/sqlite\"\n\n")
	b.WriteString("\t\"{{MODULE}}/internal/model\"\n")
	b.WriteString(")\n\n")

	b.WriteString("// SQLiteStore is a Store implementation backed by SQLite via sqlx.\n")
	b.WriteString("type SQLiteStore struct {\n")
	b.WriteString("\tdb *sqlx.DB\n")
	b.WriteString("}\n\n")

	b.WriteString("// NewSQLiteStore opens a SQLite database at dsn and returns a SQLiteStore.\n")
	b.WriteString("func NewSQLiteStore(dsn string) (*SQLiteStore, error) {\n")
	b.WriteString("\tdb, err := sqlx.Open(\"sqlite\", dsn+\"?_pragma=foreign_keys(1)\")\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\treturn nil, fmt.Errorf(\"open sqlite: %w\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\tif err := db.Ping(); err != nil {\n")
	b.WriteString("\t\treturn nil, fmt.Errorf(\"ping sqlite: %w\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn &SQLiteStore{db: db}, nil\n")
	b.WriteString("}\n\n")

	b.WriteString("// DB returns the underlying *sqlx.DB.\n")
	b.WriteString("func (s *SQLiteStore) DB() *sqlx.DB { return s.db }\n\n")

	b.WriteString("// Close releases the database connection.\n")
	b.WriteString("func (s *SQLiteStore) Close() error { return s.db.Close() }\n\n")

	for _, table := range usage.Tables() {
		schema, hasSchema := schemaMap[table]
		structName := TableToStructName(table)
		pluralName := structName + "s"

		pkType := "int64"
		pkCol := "id"
		if hasSchema {
			pkType = findPKType(schema)
			pkCol = findPKColumn(schema)
		}

		for _, method := range usage.Methods(table) {
			switch method {
			case "List":
				b.WriteString(fmt.Sprintf("func (s *SQLiteStore) List%s(ctx context.Context) ([]model.%s, error) {\n",
					pluralName, structName))
				b.WriteString(fmt.Sprintf("\tvar rows []model.%s\n", structName))
				b.WriteString(fmt.Sprintf("\terr := s.db.SelectContext(ctx, &rows, \"SELECT * FROM %s\")\n", table))
				b.WriteString("\treturn rows, err\n")
				b.WriteString("}\n\n")

			case "Get":
				b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Get%s(ctx context.Context, id %s) (*model.%s, error) {\n",
					structName, pkType, structName))
				b.WriteString(fmt.Sprintf("\tvar row model.%s\n", structName))
				b.WriteString(fmt.Sprintf("\terr := s.db.GetContext(ctx, &row, \"SELECT * FROM %s WHERE %s = ?\", id)\n",
					table, pkCol))
				b.WriteString("\tif err != nil {\n")
				b.WriteString("\t\treturn nil, err\n")
				b.WriteString("\t}\n")
				b.WriteString("\treturn &row, nil\n")
				b.WriteString("}\n\n")

			case "Create":
				if hasSchema {
					cols := nonPrimaryColumns(schema)
					colNames := columnNames(cols)
					placeholders := columnNamedPlaceholders(cols)
					b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Create%s(ctx context.Context, v *model.%s) error {\n",
						structName, structName))
					b.WriteString(fmt.Sprintf("\t_, err := s.db.NamedExecContext(ctx,\n"))
					b.WriteString(fmt.Sprintf("\t\t\"INSERT INTO %s (%s) VALUES (%s)\",\n", table, colNames, placeholders))
					b.WriteString("\t\tv)\n")
					b.WriteString("\treturn err\n")
					b.WriteString("}\n\n")
				} else {
					b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Create%s(ctx context.Context, v *model.%s) error {\n",
						structName, structName))
					b.WriteString("\t_, err := s.db.NamedExecContext(ctx, \"INSERT INTO " + table + " VALUES (:id)\", v)\n")
					b.WriteString("\treturn err\n")
					b.WriteString("}\n\n")
				}

			case "Update":
				if hasSchema {
					cols := nonPrimaryColumns(schema)
					setClauses := columnSetClauses(cols)
					b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Update%s(ctx context.Context, id %s, v *model.%s) error {\n",
						structName, pkType, structName))
					b.WriteString(fmt.Sprintf("\t_, err := s.db.NamedExecContext(ctx,\n"))
					b.WriteString(fmt.Sprintf("\t\t\"UPDATE %s SET %s WHERE %s = :%s\",\n", table, setClauses, pkCol, pkCol))
					b.WriteString("\t\tv)\n")
					b.WriteString("\treturn err\n")
					b.WriteString("}\n\n")
				} else {
					b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Update%s(ctx context.Context, id %s, v *model.%s) error {\n",
						structName, pkType, structName))
					b.WriteString("\t_, err := s.db.NamedExecContext(ctx, \"UPDATE " + table + " SET id = :id WHERE id = :id\", v)\n")
					b.WriteString("\treturn err\n")
					b.WriteString("}\n\n")
				}

			case "Delete":
				b.WriteString(fmt.Sprintf("func (s *SQLiteStore) Delete%s(ctx context.Context, id %s) error {\n",
					structName, pkType))
				b.WriteString(fmt.Sprintf("\t_, err := s.db.ExecContext(ctx, \"DELETE FROM %s WHERE %s = ?\", id)\n",
					table, pkCol))
				b.WriteString("\treturn err\n")
				b.WriteString("}\n\n")
			}
		}
	}

	return b.String()
}

// findPKType returns the Go type of the primary key column, defaulting to "int64".
func findPKType(schema manifest.Schema) string {
	for _, c := range schema.Columns {
		if c.Primary {
			return GoType(c)
		}
	}
	return "int64"
}

// findPKColumn returns the column name of the primary key, defaulting to "id".
func findPKColumn(schema manifest.Schema) string {
	for _, c := range schema.Columns {
		if c.Primary {
			return c.Name
		}
	}
	return "id"
}

// nonPrimaryColumns returns all columns that are not both primary and auto.
func nonPrimaryColumns(schema manifest.Schema) []manifest.Column {
	var cols []manifest.Column
	for _, c := range schema.Columns {
		if c.Primary && c.Auto {
			continue
		}
		cols = append(cols, c)
	}
	return cols
}

// columnNames returns a comma-separated list of column names.
func columnNames(cols []manifest.Column) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

// columnNamedPlaceholders returns a comma-separated list of :name placeholders.
func columnNamedPlaceholders(cols []manifest.Column) string {
	ph := make([]string, len(cols))
	for i, c := range cols {
		ph[i] = ":" + c.Name
	}
	return strings.Join(ph, ", ")
}

// columnSetClauses returns "name = :name, make = :make" style SET clauses.
func columnSetClauses(cols []manifest.Column) string {
	clauses := make([]string, len(cols))
	for i, c := range cols {
		clauses[i] = fmt.Sprintf("%s = :%s", c.Name, c.Name)
	}
	return strings.Join(clauses, ", ")
}
