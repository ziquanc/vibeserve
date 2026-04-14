package manifest

import (
	"strings"
	"testing"
)

func TestValidStates(t *testing.T) {
	sm := StateMachine{
		Field:   "status",
		Initial: "draft",
		Transitions: []Transition{
			{From: "draft", To: "submitted", Action: "submit"},
			{From: "submitted", To: "approved", Action: "approve"},
			{From: "submitted", To: "rejected", Action: "reject"},
		},
	}

	states := sm.ValidStates()
	if len(states) != 4 {
		t.Fatalf("expected 4 unique states, got %d: %v", len(states), states)
	}

	expected := map[string]bool{"draft": true, "submitted": true, "approved": true, "rejected": true}
	for _, s := range states {
		if !expected[s] {
			t.Errorf("unexpected state %q", s)
		}
	}
}

func TestTransitionsFrom(t *testing.T) {
	sm := StateMachine{
		Field:   "status",
		Initial: "draft",
		Transitions: []Transition{
			{From: "draft", To: "submitted", Action: "submit"},
			{From: "submitted", To: "approved", Action: "approve"},
			{From: "submitted", To: "rejected", Action: "reject"},
		},
	}

	fromSubmitted := sm.TransitionsFrom("submitted")
	if len(fromSubmitted) != 2 {
		t.Fatalf("expected 2 transitions from 'submitted', got %d", len(fromSubmitted))
	}

	fromApproved := sm.TransitionsFrom("approved")
	if len(fromApproved) != 0 {
		t.Fatalf("expected 0 transitions from 'approved', got %d", len(fromApproved))
	}

	fromDraft := sm.TransitionsFrom("draft")
	if len(fromDraft) != 1 {
		t.Fatalf("expected 1 transition from 'draft', got %d", len(fromDraft))
	}
	if fromDraft[0].Action != "submit" {
		t.Errorf("expected action 'submit', got %q", fromDraft[0].Action)
	}
}

func TestGenerateTransitionRoutes(t *testing.T) {
	sm := &StateMachine{
		Field:   "status",
		Initial: "draft",
		Transitions: []Transition{
			{From: "draft", To: "submitted", Action: "submit"},
			{From: "submitted", To: "approved", Action: "approve", Guard: &TransitionGuard{Role: "admin"}},
			{From: "submitted", To: "rejected", Action: "reject", Guard: &TransitionGuard{Condition: "row.review_count > 0"}},
		},
	}

	routes, scripts := GenerateTransitionRoutes("orders", sm)

	// 3 transition routes + 1 list transitions route = 4
	if len(routes) != 4 {
		t.Fatalf("expected 4 routes, got %d", len(routes))
	}
	if len(scripts) != 4 {
		t.Fatalf("expected 4 scripts, got %d", len(scripts))
	}

	// Check transition action routes
	expectedPaths := []string{
		"/orders/:id/submit",
		"/orders/:id/approve",
		"/orders/:id/reject",
	}
	for i, ep := range expectedPaths {
		if routes[i].Path != ep {
			t.Errorf("route[%d]: expected path %q, got %q", i, ep, routes[i].Path)
		}
		if routes[i].Method != "POST" {
			t.Errorf("route[%d]: expected method POST, got %q", i, routes[i].Method)
		}
	}

	// Check the transitions list route
	listRoute := routes[3]
	if listRoute.Path != "/orders/:id/transitions" {
		t.Errorf("expected transitions list path '/orders/:id/transitions', got %q", listRoute.Path)
	}
	if listRoute.Method != "GET" {
		t.Errorf("expected method GET for transitions list, got %q", listRoute.Method)
	}

	// Check role guard in approve script
	approveScript := scripts[1].Code
	if !strings.Contains(approveScript, `request.auth()`) {
		t.Error("approve script should check request.auth() for role guard")
	}
	if !strings.Contains(approveScript, `"admin"`) {
		t.Error("approve script should check for admin role")
	}

	// Check condition guard in reject script
	rejectScript := scripts[2].Code
	if !strings.Contains(rejectScript, "row.review_count > 0") {
		t.Error("reject script should check condition guard")
	}

	// Check that submit script has no guard checks
	submitScript := scripts[0].Code
	if strings.Contains(submitScript, "request.auth()") {
		t.Error("submit script should not check auth (no guard)")
	}

	// Check script names reference singularized table
	if scripts[0].Name != "order_submit" {
		t.Errorf("expected script name 'order_submit', got %q", scripts[0].Name)
	}

	// Check transitions list script
	listScript := scripts[3].Code
	if !strings.Contains(listScript, "transitions") {
		t.Error("transitions list script should build transitions array")
	}
}

func TestGenerateTransitionRoutes_NoGuards(t *testing.T) {
	sm := &StateMachine{
		Field:   "status",
		Initial: "pending",
		Transitions: []Transition{
			{From: "pending", To: "active", Action: "activate"},
			{From: "active", To: "archived", Action: "archive"},
		},
	}

	routes, scripts := GenerateTransitionRoutes("items", sm)

	// 2 transition routes + 1 list route = 3
	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}
	if len(scripts) != 3 {
		t.Fatalf("expected 3 scripts, got %d", len(scripts))
	}

	// Verify no guard checks in any transition script
	for i := 0; i < 2; i++ {
		code := scripts[i].Code
		if strings.Contains(code, "request.auth()") {
			t.Errorf("script[%d] should not check auth when no guard is set", i)
		}
		if strings.Contains(code, "guard") {
			t.Errorf("script[%d] should not contain guard logic when no guard is set", i)
		}
	}

	// Verify correct paths
	if routes[0].Path != "/items/:id/activate" {
		t.Errorf("expected '/items/:id/activate', got %q", routes[0].Path)
	}
	if routes[1].Path != "/items/:id/archive" {
		t.Errorf("expected '/items/:id/archive', got %q", routes[1].Path)
	}

	// Verify scripts contain the essential state transition logic
	for i := 0; i < 2; i++ {
		code := scripts[i].Code
		if !strings.Contains(code, "db.query_one") {
			t.Errorf("script[%d] should query for the row", i)
		}
		if !strings.Contains(code, "response.fail") {
			t.Errorf("script[%d] should handle failure cases", i)
		}
		if !strings.Contains(code, "response.json") {
			t.Errorf("script[%d] should return JSON response", i)
		}
	}
}
