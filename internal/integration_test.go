package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

func setupCarRentalServer(t *testing.T) http.Handler {
	t.Helper()

	m, err := manifest.LoadFromFile("../testdata/car_rental_manifest.json")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if err := manifest.Validate(m); err != nil {
		t.Fatalf("validate: %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.ApplySchemas(m.Schemas); err != nil {
		t.Fatalf("apply schemas: %v", err)
	}
	for _, seed := range m.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			t.Fatalf("seed %s: %v", seed.Table, err)
		}
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
	}

	rt := runtime.New(s, engine.NewBus())
	return router.NewHandler(trie, router.MapScriptResolver(scripts), rt, true)
}

func TestIntegration_ListVehicles(t *testing.T) {
	h := setupCarRentalServer(t)
	req := httptest.NewRequest("GET", "/vehicles", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var vehicles []map[string]any
	json.NewDecoder(rec.Body).Decode(&vehicles)
	if len(vehicles) != 3 {
		t.Fatalf("expected 3 vehicles, got %d", len(vehicles))
	}
	// Check Malaysian context
	found := false
	for _, v := range vehicles {
		if v["make"] == "Perodua" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Perodua in vehicle list")
	}
}

func TestIntegration_GetVehicle(t *testing.T) {
	h := setupCarRentalServer(t)
	req := httptest.NewRequest("GET", "/vehicles/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var vehicle map[string]any
	json.NewDecoder(rec.Body).Decode(&vehicle)
	if vehicle["make"] != "Perodua" {
		t.Errorf("expected Perodua, got %v", vehicle["make"])
	}
}

func TestIntegration_GetVehicle_NotFound(t *testing.T) {
	h := setupCarRentalServer(t)
	req := httptest.NewRequest("GET", "/vehicles/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestIntegration_CreateBooking_WithDiscount(t *testing.T) {
	h := setupCarRentalServer(t)

	// Perodua Myvi (89 MYR/day) for 10 days → 20% discount: 89 * 0.8 * 10 = 712.0
	body := `{"vehicle_id": 1, "customer": "Ahmad", "start_date": "2026-04-10", "end_date": "2026-04-20"}`
	req := httptest.NewRequest("POST", "/bookings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var booking map[string]any
	json.NewDecoder(rec.Body).Decode(&booking)

	totalPrice, ok := booking["total_price"].(float64)
	if !ok {
		t.Fatalf("expected float64 total_price, got %T: %v", booking["total_price"], booking["total_price"])
	}
	if totalPrice != 712.0 {
		t.Errorf("expected total_price 712.0 (10 days * 89 * 0.8), got %v", totalPrice)
	}
	if booking["status"] != "confirmed" {
		t.Errorf("expected status 'confirmed', got %v", booking["status"])
	}

	// Verify vehicle is now unavailable
	req2 := httptest.NewRequest("GET", "/vehicles", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	var vehicles []map[string]any
	json.NewDecoder(rec2.Body).Decode(&vehicles)
	if len(vehicles) != 2 {
		t.Errorf("expected 2 available vehicles after booking, got %d", len(vehicles))
	}
}

func TestIntegration_CreateBooking_NoDiscount(t *testing.T) {
	h := setupCarRentalServer(t)

	// Proton X50 (149 MYR/day) for 3 days → no discount: 149 * 3 = 447.0
	body := `{"vehicle_id": 2, "customer": "Siti", "start_date": "2026-04-10", "end_date": "2026-04-13"}`
	req := httptest.NewRequest("POST", "/bookings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var booking map[string]any
	json.NewDecoder(rec.Body).Decode(&booking)

	totalPrice, ok := booking["total_price"].(float64)
	if !ok {
		t.Fatalf("expected float64, got %T: %v", booking["total_price"], booking["total_price"])
	}
	if totalPrice != 447.0 {
		t.Errorf("expected 447.0 (3 days * 149, no discount), got %v", totalPrice)
	}
}

func TestIntegration_CORS(t *testing.T) {
	h := setupCarRentalServer(t)
	req := httptest.NewRequest("OPTIONS", "/vehicles", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Errorf("expected 204 for preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}
