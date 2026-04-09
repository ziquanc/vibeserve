package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateOpenAPI(t *testing.T) {
	m := &manifest.Manifest{
		Name:        "Car Rental API",
		Description: "Rent cars in Malaysia",
		Version:     "1.0",
		Schemas: []manifest.Schema{
			{
				Table: "vehicles",
				Columns: []manifest.Column{
					{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
					{Name: "make", Type: "TEXT", Required: true},
					{Name: "daily_rate", Type: "REAL", Required: true},
					{Name: "available", Type: "BOOLEAN"},
				},
			},
		},
		Routes: []manifest.Route{
			{Path: "/vehicles", Method: "GET", Description: "List all vehicles", Script: "list_vehicles", ResponseType: "array"},
			{Path: "/vehicles/:id", Method: "GET", Description: "Get vehicle by ID", Script: "get_vehicle", ResponseType: "object"},
			{Path: "/bookings", Method: "POST", Description: "Create booking", Script: "create_booking",
				RequestBody: map[string]string{"vehicle_id": "INTEGER", "customer": "TEXT"}, ResponseType: "object"},
		},
	}

	yaml := GenerateOpenAPI(m)

	if !strings.Contains(yaml, "openapi: \"3.0.3\"") {
		t.Error("missing openapi version")
	}
	if !strings.Contains(yaml, "title: \"Car Rental API\"") {
		t.Error("missing title")
	}
	if !strings.Contains(yaml, "/vehicles:") {
		t.Error("missing /vehicles path")
	}
	if !strings.Contains(yaml, "/vehicles/{id}:") {
		t.Error("missing /vehicles/{id} path (should convert :id to {id})")
	}
	if strings.Contains(yaml, "/vehicles/:id") {
		t.Error(":id should be converted to {id}")
	}
	if !strings.Contains(yaml, "Vehicle:") {
		t.Error("missing Vehicle schema in components")
	}
	if !strings.Contains(yaml, "requestBody:") {
		t.Error("missing requestBody for POST route")
	}
}
