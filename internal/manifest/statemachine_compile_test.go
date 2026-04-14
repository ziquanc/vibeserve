package manifest

import (
	"testing"

	"github.com/d5/tengo/v2"
)

func TestStateMachineScriptsCompile(t *testing.T) {
	sm := &StateMachine{
		Field:   "status",
		Initial: "draft",
		Transitions: []Transition{
			{From: "draft", To: "submitted", Action: "submit", Guard: &TransitionGuard{Condition: "total > 0"}},
			{From: "submitted", To: "approved", Action: "approve", Guard: &TransitionGuard{Role: "admin"}},
			{From: "submitted", To: "rejected", Action: "reject", Guard: &TransitionGuard{Role: "admin"}},
		},
	}

	routes, scripts := GenerateTransitionRoutes("orders", sm)
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
