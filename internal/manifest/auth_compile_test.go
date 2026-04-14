package manifest

import (
	"testing"

	"github.com/d5/tengo/v2"
)

func TestAuthScriptsCompile(t *testing.T) {
	routes, scripts := GenerateAuthRoutes("users")
	stdlibNames := []string{"db", "request", "response", "date", "crypto", "log", "auth"}

	for i, s := range scripts {
		script := tengo.NewScript([]byte(s.Code))
		for _, name := range stdlibNames {
			_ = script.Add(name, map[string]interface{}{})
		}
		_, err := script.Compile()
		if err != nil {
			t.Errorf("script %q (%s %s) failed to compile: %v", s.Name, routes[i].Method, routes[i].Path, err)
		}
	}
}
