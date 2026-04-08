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

// Note: ScriptEvaluator interface deferred to Phase 2.
// In Phase 1, the handler uses *runtime.Runtime directly.
