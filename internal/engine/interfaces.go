package engine

import (
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// DataStore provides database operations for the Tengo stdlib.
type DataStore interface {
	ApplySchemas(schemas []manifest.Schema) error
	Seed(table string, rows []map[string]any) error
	Query(sql string, params []any) ([]map[string]any, error)
	QueryOne(sql string, params []any) (map[string]any, error)
	Insert(table string, data map[string]any) (map[string]any, error)
	Update(table string, id any, data map[string]any) (map[string]any, error)
	Delete(table string, id any) (bool, error)
	Count(table string) (int, error)
	Close() error
}

// RouteTrie provides route registration and removal operations.
// This interface is satisfied by *router.Trie but avoids an import cycle
// since router/handler.go imports runtime, which imports engine.
type RouteTrie interface {
	Insert(method, path, script string)
	Remove(method, path string)
}

// SchemaStore provides the schema migration and seeding operations used by the Engine.
// This interface is satisfied by *store.Store but avoids an import cycle
// since store.go imports engine for the DataStore interface compile check.
type SchemaStore interface {
	ApplySchemas(schemas []manifest.Schema) error
	AddColumn(table string, col manifest.Column) error
	Seed(table string, rows []map[string]any) error
	DSN() string
	Close() error
}

// Note: ScriptEvaluator interface deferred to Phase 2.
// In Phase 1, the handler uses *runtime.Runtime directly.
