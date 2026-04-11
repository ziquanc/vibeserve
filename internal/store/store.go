package store

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // register "sqlite" driver

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// validTableName matches only alphanumeric and underscore characters.
var validTableName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ValidateTableName rejects table names that could enable SQL injection.
func ValidateTableName(table string) error {
	if !validTableName.MatchString(table) {
		return fmt.Errorf("invalid table name: %q (only [a-zA-Z0-9_] allowed)", table)
	}
	return nil
}

// ValidateColumnName rejects column names that could enable SQL injection.
func ValidateColumnName(column string) error {
	if !validTableName.MatchString(column) {
		return fmt.Errorf("invalid column name: %q (only [a-zA-Z0-9_] allowed)", column)
	}
	return nil
}

// Compile-time check that *Store satisfies engine.DataStore.
var _ engine.DataStore = (*Store)(nil)

// Store is the SQLite-backed DataStore implementation.
type Store struct {
	db          *sql.DB
	dsn         string
	boolColumns map[string]bool // key: "table.column"
}

// New opens a SQLite database at dsn, enables foreign keys and WAL mode.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	return &Store{
		db:          db,
		dsn:         dsn,
		boolColumns: make(map[string]bool),
	}, nil
}

// ApplySchemas creates tables from manifest schemas and records boolean columns.
func (s *Store) ApplySchemas(schemas []manifest.Schema) error {
	for _, schema := range schemas {
		if err := ValidateTableName(schema.Table); err != nil {
			return err
		}
		ddl := BuildCreateTableSQL(schema)
		if _, err := s.db.Exec(ddl); err != nil {
			return fmt.Errorf("create table %s: %w", schema.Table, err)
		}
		// Track boolean columns for type mapping
		for _, col := range schema.Columns {
			if strings.ToUpper(col.Type) == "BOOLEAN" {
				key := schema.Table + "." + col.Name
				s.boolColumns[key] = true
			}
		}
	}
	return nil
}

// AddColumn executes an ALTER TABLE ADD COLUMN statement for the given column.
func (s *Store) AddColumn(table string, col manifest.Column) error {
	if err := ValidateTableName(table); err != nil {
		return err
	}
	ddl := BuildAddColumnSQL(table, col)
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, col.Name, err)
	}
	if strings.ToUpper(col.Type) == "BOOLEAN" {
		s.boolColumns[table+"."+col.Name] = true
	}
	return nil
}

// DSN returns the data source name used to open the database.
func (s *Store) DSN() string {
	return s.dsn
}

// Seed inserts multiple rows into a table (used for initial data seeding).
func (s *Store) Seed(table string, rows []map[string]any) error {
	if err := ValidateTableName(table); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := s.Insert(table, row); err != nil {
			return fmt.Errorf("seed %s: %w", table, err)
		}
	}
	return nil
}

// Query executes a parameterized SELECT and returns all matching rows.
func (s *Store) Query(query string, params []any) ([]map[string]any, error) {
	rows, err := s.db.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// QueryOne executes a parameterized SELECT and returns the first matching row,
// or nil if no rows match (not an error).
func (s *Store) QueryOne(query string, params []any) (map[string]any, error) {
	rows, err := s.db.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("query one: %w", err)
	}
	defer rows.Close()

	results, err := s.scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// Insert inserts a row into table and returns the full inserted row.
func (s *Store) Insert(table string, data map[string]any) (map[string]any, error) {
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("insert: no data provided")
	}

	cols := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]any, 0, len(data))

	for col, val := range data {
		cols = append(cols, col)
		placeholders = append(placeholders, "?")
		values = append(values, s.convertForWrite(table, col, val))
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	result, err := s.db.Exec(query, values...)
	if err != nil {
		return nil, fmt.Errorf("insert into %s: %w", table, err)
	}

	rowid, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	// Re-read the full row using rowid
	rows, err := s.db.Query(fmt.Sprintf("SELECT * FROM %s WHERE rowid = ?", table), rowid)
	if err != nil {
		return nil, fmt.Errorf("re-read after insert: %w", err)
	}
	defer rows.Close()

	results, err := s.scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("insert: row not found after insert")
	}
	return results[0], nil
}

// Update updates columns in table where id matches and returns the full updated row.
func (s *Store) Update(table string, id any, data map[string]any) (map[string]any, error) {
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("update: no data provided")
	}

	setClauses := make([]string, 0, len(data))
	values := make([]any, 0, len(data)+1)

	for col, val := range data {
		setClauses = append(setClauses, col+" = ?")
		values = append(values, s.convertForWrite(table, col, val))
	}
	values = append(values, id)

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = ?",
		table,
		strings.Join(setClauses, ", "),
	)

	if _, err := s.db.Exec(query, values...); err != nil {
		return nil, fmt.Errorf("update %s: %w", table, err)
	}

	// Re-read the full updated row
	rows, err := s.db.Query(fmt.Sprintf("SELECT * FROM %s WHERE id = ?", table), id)
	if err != nil {
		return nil, fmt.Errorf("re-read after update: %w", err)
	}
	defer rows.Close()

	results, err := s.scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("update: row not found after update")
	}
	return results[0], nil
}

// Delete removes the row with the given id from table.
// Returns true if a row was deleted, false if no row matched.
func (s *Store) Delete(table string, id any) (bool, error) {
	if err := ValidateTableName(table); err != nil {
		return false, err
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
	result, err := s.db.Exec(query, id)
	if err != nil {
		return false, fmt.Errorf("delete from %s: %w", table, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return affected > 0, nil
}

// Count returns the number of rows in a table.
func (s *Store) Count(table string) (int, error) {
	if err := ValidateTableName(table); err != nil {
		return 0, err
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	var count int
	if err := s.db.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return count, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// scanRows reads sql.Rows into a slice of maps and applies boolean type mapping.
func (s *Store) scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	// Try to detect the table name from column metadata for bool mapping.
	// We'll use a fallback: check all known bool columns across all tables.
	var result []map[string]any

	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			val := values[i]
			// Apply boolean conversion: check if any table has this column as BOOLEAN
			if s.isBoolColumn(col) {
				val = convertToBool(val)
			}
			row[col] = val
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return result, nil
}

// isBoolColumn returns true if any registered table has this column name as BOOLEAN.
func (s *Store) isBoolColumn(colName string) bool {
	// Check all tables — key format is "table.column"
	for key := range s.boolColumns {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) == 2 && parts[1] == colName {
			return true
		}
	}
	return false
}

// convertToBool converts a SQLite INTEGER value to Go bool.
func convertToBool(val any) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case int64:
		return v != 0
	case bool:
		return v
	case int:
		return v != 0
	default:
		return false
	}
}

// convertForWrite converts a Go bool to int64 for SQLite storage.
func (s *Store) convertForWrite(table, col string, val any) any {
	key := table + "." + col
	if s.boolColumns[key] {
		if b, ok := val.(bool); ok {
			if b {
				return int64(1)
			}
			return int64(0)
		}
	}
	return val
}

// TableInfo describes a database table and its row count.
type TableInfo struct {
	Name     string       `json:"name"`
	Columns  []ColumnInfo `json:"columns"`
	RowCount int          `json:"row_count"`
}

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
	PK      bool   `json:"pk"`
}

// Tables returns metadata for all user-created tables (excludes sqlite_ internals).
func (s *Store) Tables() ([]TableInfo, error) {
	// First, collect all table names (close the cursor before running nested queries).
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close table list rows: %w", err)
	}

	// Now query columns and row counts for each table.
	var tables []TableInfo
	for _, name := range names {
		ti := TableInfo{Name: name}

		// Get column info via PRAGMA
		pragmaRows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", name))
		if err != nil {
			return nil, fmt.Errorf("pragma table_info(%s): %w", name, err)
		}
		for pragmaRows.Next() {
			var cid int
			var colName, colType string
			var notNull, pk int
			var dfltValue sql.NullString
			if err := pragmaRows.Scan(&cid, &colName, &colType, &notNull, &dfltValue, &pk); err != nil {
				pragmaRows.Close()
				return nil, fmt.Errorf("scan column info: %w", err)
			}
			ti.Columns = append(ti.Columns, ColumnInfo{
				Name:    colName,
				Type:    colType,
				NotNull: notNull == 1,
				PK:      pk == 1,
			})
		}
		pragmaRows.Close()

		// Get row count
		count, err := s.Count(name)
		if err != nil {
			count = 0
		}
		ti.RowCount = count

		tables = append(tables, ti)
	}
	return tables, nil
}
