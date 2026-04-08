package store

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestBuildCreateTableSQL_Simple(t *testing.T) {
	schema := manifest.Schema{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
		},
	}
	sql := BuildCreateTableSQL(schema)
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS users") {
		t.Errorf("missing CREATE TABLE: %s", sql)
	}
	if !strings.Contains(sql, "id INTEGER PRIMARY KEY AUTOINCREMENT") {
		t.Errorf("missing primary key autoincrement: %s", sql)
	}
	if !strings.Contains(sql, "name TEXT NOT NULL") {
		t.Errorf("missing NOT NULL: %s", sql)
	}
}

func TestBuildCreateTableSQL_TypeMapping(t *testing.T) {
	schema := manifest.Schema{
		Table: "things",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "flag", Type: "BOOLEAN"},
			{Name: "score", Type: "REAL"},
			{Name: "label", Type: "TEXT"},
			{Name: "day", Type: "DATE"},
			{Name: "created_at", Type: "DATETIME"},
		},
	}
	sql := BuildCreateTableSQL(schema)
	if !strings.Contains(sql, "flag INTEGER") {
		t.Errorf("BOOLEAN should map to INTEGER: %s", sql)
	}
	if !strings.Contains(sql, "score REAL") {
		t.Errorf("REAL should map to REAL: %s", sql)
	}
	if !strings.Contains(sql, "label TEXT") {
		t.Errorf("TEXT should map to TEXT: %s", sql)
	}
	if !strings.Contains(sql, "day TEXT") {
		t.Errorf("DATE should map to TEXT: %s", sql)
	}
	if !strings.Contains(sql, "created_at TEXT") {
		t.Errorf("DATETIME should map to TEXT: %s", sql)
	}
}

func TestBuildCreateTableSQL_PrimaryKey_NoAuto(t *testing.T) {
	schema := manifest.Schema{
		Table: "kv",
		Columns: []manifest.Column{
			{Name: "key", Type: "TEXT", Primary: true},
			{Name: "value", Type: "TEXT"},
		},
	}
	sql := BuildCreateTableSQL(schema)
	if !strings.Contains(sql, "key TEXT PRIMARY KEY") {
		t.Errorf("missing PRIMARY KEY without AUTOINCREMENT: %s", sql)
	}
	if strings.Contains(sql, "AUTOINCREMENT") {
		t.Errorf("should not have AUTOINCREMENT: %s", sql)
	}
}

func TestBuildCreateTableSQL_Unique(t *testing.T) {
	schema := manifest.Schema{
		Table: "accounts",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "email", Type: "TEXT", Required: true, Unique: true},
		},
	}
	sql := BuildCreateTableSQL(schema)
	if !strings.Contains(sql, "email TEXT NOT NULL UNIQUE") {
		t.Errorf("missing NOT NULL UNIQUE: %s", sql)
	}
}

func TestBuildCreateTableSQL_Defaults(t *testing.T) {
	schema := manifest.Schema{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "active", Type: "BOOLEAN", Default: true},
			{Name: "disabled", Type: "BOOLEAN", Default: false},
			{Name: "label", Type: "TEXT", Default: "hello"},
			{Name: "count", Type: "INTEGER", Default: float64(0)},
			{Name: "created_at", Type: "DATETIME", Default: "NOW"},
		},
	}
	sql := BuildCreateTableSQL(schema)
	if !strings.Contains(sql, "active INTEGER DEFAULT 1") {
		t.Errorf("bool true default should be 1: %s", sql)
	}
	if !strings.Contains(sql, "disabled INTEGER DEFAULT 0") {
		t.Errorf("bool false default should be 0: %s", sql)
	}
	if !strings.Contains(sql, `label TEXT DEFAULT 'hello'`) {
		t.Errorf("string default should be quoted: %s", sql)
	}
	if !strings.Contains(sql, "count INTEGER DEFAULT 0") {
		t.Errorf("numeric default should be unquoted: %s", sql)
	}
	if !strings.Contains(sql, "created_at TEXT DEFAULT CURRENT_TIMESTAMP") {
		t.Errorf("NOW default should become CURRENT_TIMESTAMP: %s", sql)
	}
}

func TestBuildCreateTableSQL_References(t *testing.T) {
	schema := manifest.Schema{
		Table: "posts",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "user_id", Type: "INTEGER", Required: true, References: "users(id)"},
		},
	}
	sql := BuildCreateTableSQL(schema)
	if !strings.Contains(sql, "user_id INTEGER NOT NULL REFERENCES users(id)") {
		t.Errorf("missing REFERENCES clause: %s", sql)
	}
}
