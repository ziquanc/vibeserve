package runtime

import (
	"testing"

	tengo "github.com/d5/tengo/v2"
)

// TestStdlibModulesAreImmutableMaps verifies that each module builder returns a *tengo.ImmutableMap.
func TestStdlibModulesAreImmutableMaps(t *testing.T) {
	store, bus, rc, capture := newTestDeps(t)
	_ = store
	_ = bus
	_ = rc
	_ = capture

	modules := map[string]tengo.Object{
		"db":       newDBModule(store),
		"request":  newRequestModule(rc),
		"response": newResponseModule(capture),
		"date":     newDateModule(),
		"crypto":   newCryptoModule(),
		"log":      newLogModule(bus),
	}

	for name, mod := range modules {
		if _, ok := mod.(*tengo.ImmutableMap); !ok {
			t.Errorf("module %q is not *tengo.ImmutableMap, got %T", name, mod)
		}
	}
}

// TestDBModuleHasExpectedKeys checks that the db module exposes the required functions.
func TestDBModuleHasExpectedKeys(t *testing.T) {
	store, _, _, _ := newTestDeps(t)
	mod := newDBModule(store).(*tengo.ImmutableMap)

	requiredKeys := []string{"query", "query_one", "insert", "update", "delete", "count"}
	for _, k := range requiredKeys {
		if _, ok := mod.Value[k]; !ok {
			t.Errorf("db module missing key %q", k)
		}
	}
}

// TestRequestModuleHasExpectedKeys checks that the request module exposes the required functions.
func TestRequestModuleHasExpectedKeys(t *testing.T) {
	_, _, rc, _ := newTestDeps(t)
	mod := newRequestModule(rc).(*tengo.ImmutableMap)

	requiredKeys := []string{"param", "query", "body", "header", "method", "auth"}
	for _, k := range requiredKeys {
		if _, ok := mod.Value[k]; !ok {
			t.Errorf("request module missing key %q", k)
		}
	}
}

// TestResponseModuleHasExpectedKeys checks that the response module exposes the required functions.
// Note: "error" is a Tengo keyword, so the function is exposed as "fail".
func TestResponseModuleHasExpectedKeys(t *testing.T) {
	_, _, _, capture := newTestDeps(t)
	mod := newResponseModule(capture).(*tengo.ImmutableMap)

	requiredKeys := []string{"json", "fail", "header", "redirect"}
	for _, k := range requiredKeys {
		if _, ok := mod.Value[k]; !ok {
			t.Errorf("response module missing key %q", k)
		}
	}
}

// TestDateModuleHasExpectedKeys checks date module keys.
func TestDateModuleHasExpectedKeys(t *testing.T) {
	mod := newDateModule().(*tengo.ImmutableMap)

	requiredKeys := []string{"now", "diff_days", "add_days", "format"}
	for _, k := range requiredKeys {
		if _, ok := mod.Value[k]; !ok {
			t.Errorf("date module missing key %q", k)
		}
	}
}

// TestCryptoModuleHasExpectedKeys checks crypto module keys.
func TestCryptoModuleHasExpectedKeys(t *testing.T) {
	mod := newCryptoModule().(*tengo.ImmutableMap)

	requiredKeys := []string{"hash", "uuid", "random"}
	for _, k := range requiredKeys {
		if _, ok := mod.Value[k]; !ok {
			t.Errorf("crypto module missing key %q", k)
		}
	}
}

// TestLogModuleHasExpectedKeys checks log module keys.
func TestLogModuleHasExpectedKeys(t *testing.T) {
	_, bus, _, _ := newTestDeps(t)
	mod := newLogModule(bus).(*tengo.ImmutableMap)

	requiredKeys := []string{"info", "warn", "error"}
	for _, k := range requiredKeys {
		if _, ok := mod.Value[k]; !ok {
			t.Errorf("log module missing key %q", k)
		}
	}
}
