package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateDatabaseJS creates src/models/database.js with better-sqlite3 setup
// and helper CRUD functions for each schema table.
func GenerateDatabaseJS(schemas []manifest.Schema) string {
	var b strings.Builder

	b.WriteString(`const Database = require('better-sqlite3');
const path = require('path');

const DB_PATH = process.env.DATABASE_PATH || path.join(__dirname, '..', 'data', 'state.db');

let db;

/**
 * Initialize the database connection and create tables if they don't exist.
 */
function initDB() {
  db = new Database(DB_PATH);

  // Enable WAL mode for better concurrent read performance
  db.pragma('journal_mode = WAL');
  // Enable foreign keys
  db.pragma('foreign_keys = ON');

`)

	// Generate CREATE TABLE statements for each schema
	for _, schema := range schemas {
		b.WriteString(fmt.Sprintf("  // Create %s table\n", schema.Table))
		b.WriteString(generateCreateTable(schema))
		b.WriteString("\n")
	}

	b.WriteString(`  console.log('Database tables ensured');
  return db;
}

/**
 * Get the database instance (initializes if needed).
 */
function getDB() {
  if (!db) {
    initDB();
  }
  return db;
}

`)

	// Generate CRUD helper functions for each table
	for _, schema := range schemas {
		b.WriteString(generateTableHelpers(schema))
		b.WriteString("\n")
	}

	// Export
	b.WriteString("module.exports = {\n")
	b.WriteString("  initDB,\n")
	b.WriteString("  getDB,\n")
	for _, schema := range schemas {
		structName := TableToStructName(schema.Table)
		pluralName := structName + "s"
		b.WriteString(fmt.Sprintf("  list%s,\n", pluralName))
		b.WriteString(fmt.Sprintf("  get%s,\n", structName))
		b.WriteString(fmt.Sprintf("  create%s,\n", structName))
		b.WriteString(fmt.Sprintf("  update%s,\n", structName))
		b.WriteString(fmt.Sprintf("  delete%s,\n", structName))
	}
	b.WriteString("};\n")

	return b.String()
}

// generateCreateTable produces a CREATE TABLE IF NOT EXISTS statement.
func generateCreateTable(schema manifest.Schema) string {
	var colDefs []string
	for _, col := range schema.Columns {
		def := col.Name + " " + col.Type
		if col.Primary {
			def += " PRIMARY KEY"
		}
		if col.Auto {
			def += " AUTOINCREMENT"
		}
		if col.Required {
			def += " NOT NULL"
		}
		if col.Unique {
			def += " UNIQUE"
		}
		if col.Default != nil {
			switch v := col.Default.(type) {
			case string:
				def += fmt.Sprintf(" DEFAULT '%s'", v)
			default:
				def += fmt.Sprintf(" DEFAULT %v", v)
			}
		}
		if col.References != "" {
			def += fmt.Sprintf(" REFERENCES %s", col.References)
		}
		colDefs = append(colDefs, def)
	}

	return fmt.Sprintf("  db.exec(`CREATE TABLE IF NOT EXISTS %s (\n    %s\n  )`);\n",
		schema.Table, strings.Join(colDefs, ",\n    "))
}

// generateTableHelpers generates list/get/create/update/delete functions for a table.
func generateTableHelpers(schema manifest.Schema) string {
	var b strings.Builder
	structName := TableToStructName(schema.Table)
	pluralName := structName + "s"
	pkCol := findPKColumn(schema)

	// listXxx
	b.WriteString(fmt.Sprintf("function list%s() {\n", pluralName))
	b.WriteString("  const database = getDB();\n")
	b.WriteString(fmt.Sprintf("  return database.prepare('SELECT * FROM %s').all();\n", schema.Table))
	b.WriteString("}\n\n")

	// getXxx
	b.WriteString(fmt.Sprintf("function get%s(id) {\n", structName))
	b.WriteString("  const database = getDB();\n")
	b.WriteString(fmt.Sprintf("  return database.prepare('SELECT * FROM %s WHERE %s = ?').get(id);\n", schema.Table, pkCol))
	b.WriteString("}\n\n")

	// createXxx
	nonPKCols := nonAutoPrimaryColumns(schema)
	if len(nonPKCols) > 0 {
		colNames := columnNames(nonPKCols)
		placeholders := make([]string, len(nonPKCols))
		paramNames := make([]string, len(nonPKCols))
		for i, col := range nonPKCols {
			placeholders[i] = "?"
			paramNames[i] = "data." + col.Name
		}
		b.WriteString(fmt.Sprintf("function create%s(data) {\n", structName))
		b.WriteString("  const database = getDB();\n")
		b.WriteString(fmt.Sprintf("  const stmt = database.prepare('INSERT INTO %s (%s) VALUES (%s)');\n",
			schema.Table, colNames, strings.Join(placeholders, ", ")))
		b.WriteString(fmt.Sprintf("  const result = stmt.run(%s);\n", strings.Join(paramNames, ", ")))
		b.WriteString("  return { id: result.lastInsertRowid, ...data };\n")
	} else {
		b.WriteString(fmt.Sprintf("function create%s(data) {\n", structName))
		b.WriteString("  const database = getDB();\n")
		b.WriteString(fmt.Sprintf("  const result = database.prepare('INSERT INTO %s DEFAULT VALUES').run();\n", schema.Table))
		b.WriteString("  return { id: result.lastInsertRowid, ...data };\n")
	}
	b.WriteString("}\n\n")

	// updateXxx
	nonPKUpdateCols := nonPrimaryColumns(schema)
	if len(nonPKUpdateCols) > 0 {
		setClauses := make([]string, len(nonPKUpdateCols))
		paramNames := make([]string, len(nonPKUpdateCols)+1)
		for i, col := range nonPKUpdateCols {
			setClauses[i] = col.Name + " = ?"
			paramNames[i] = "data." + col.Name
		}
		paramNames[len(nonPKUpdateCols)] = "id"
		b.WriteString(fmt.Sprintf("function update%s(id, data) {\n", structName))
		b.WriteString("  const database = getDB();\n")
		b.WriteString(fmt.Sprintf("  database.prepare('UPDATE %s SET %s WHERE %s = ?').run(%s);\n",
			schema.Table, strings.Join(setClauses, ", "), pkCol, strings.Join(paramNames, ", ")))
		b.WriteString(fmt.Sprintf("  return get%s(id);\n", structName))
	} else {
		b.WriteString(fmt.Sprintf("function update%s(id, data) {\n", structName))
		b.WriteString("  const database = getDB();\n")
		b.WriteString(fmt.Sprintf("  database.prepare('UPDATE %s SET %s = %s WHERE %s = ?').run(id);\n",
			schema.Table, pkCol, pkCol, pkCol))
		b.WriteString(fmt.Sprintf("  return get%s(id);\n", structName))
	}
	b.WriteString("}\n\n")

	// deleteXxx
	b.WriteString(fmt.Sprintf("function delete%s(id) {\n", structName))
	b.WriteString("  const database = getDB();\n")
	b.WriteString(fmt.Sprintf("  database.prepare('DELETE FROM %s WHERE %s = ?').run(id);\n", schema.Table, pkCol))
	b.WriteString("}\n")

	return b.String()
}

// nonAutoPrimaryColumns returns columns that are NOT (primary AND auto).
// These are the columns whose values should be provided during INSERT.
func nonAutoPrimaryColumns(schema manifest.Schema) []manifest.Column {
	var cols []manifest.Column
	for _, c := range schema.Columns {
		if c.Primary && c.Auto {
			continue
		}
		cols = append(cols, c)
	}
	return cols
}
