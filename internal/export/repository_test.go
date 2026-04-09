package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestAnalyzeRouteUsage(t *testing.T) {
	routes := []manifest.Route{
		{Path: "/vehicles", Method: "GET", Script: "list_vehicles", ResponseType: "array"},
		{Path: "/vehicles/:id", Method: "GET", Script: "get_vehicle", ResponseType: "object"},
		{Path: "/bookings", Method: "POST", Script: "create_booking", ResponseType: "object"},
	}
	scripts := []manifest.Script{
		{Name: "list_vehicles", Code: "result := db.query(\"SELECT * FROM vehicles WHERE available = ?\", [true])\nresponse.json(result)"},
		{Name: "get_vehicle", Code: "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM vehicles WHERE id = ?\", [id])"},
		{Name: "create_booking", Code: "body := request.body()\nbooking := db.insert(\"bookings\", {})\ndb.update(\"vehicles\", vehicle.id, {})"},
	}

	usage := AnalyzeRouteUsage(routes, scripts)

	if !usage.HasMethod("vehicles", "List") {
		t.Error("expected List method for vehicles")
	}
	if !usage.HasMethod("vehicles", "Get") {
		t.Error("expected Get method for vehicles")
	}
	if !usage.HasMethod("bookings", "Create") {
		t.Error("expected Create method for bookings")
	}
	if !usage.HasMethod("vehicles", "Update") {
		t.Error("expected Update method for vehicles")
	}
}

func TestGenerateStoreInterface(t *testing.T) {
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "make", Type: "TEXT"},
		}},
	}
	usage := &RouteUsage{
		methods: map[string]map[string]bool{
			"vehicles": {"List": true, "Get": true},
		},
	}

	code := GenerateStoreInterface(schemas, usage)

	if !strings.Contains(code, "package repository") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "type Store interface") {
		t.Error("missing Store interface")
	}
	if !strings.Contains(code, "ListVehicles(") {
		t.Error("missing ListVehicles method")
	}
	if !strings.Contains(code, "GetVehicle(") {
		t.Error("missing GetVehicle method")
	}
	if !strings.Contains(code, "DB() *sqlx.DB") {
		t.Error("missing DB() accessor")
	}
}

func TestGenerateSQLiteStore(t *testing.T) {
	schemas := []manifest.Schema{
		{Table: "vehicles", Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "make", Type: "TEXT"},
		}},
	}
	usage := &RouteUsage{
		methods: map[string]map[string]bool{
			"vehicles": {"List": true, "Get": true},
		},
	}

	code := GenerateSQLiteStore(schemas, usage)

	if !strings.Contains(code, "package repository") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "SQLiteStore") {
		t.Error("missing SQLiteStore struct")
	}
	if !strings.Contains(code, "NewSQLiteStore") {
		t.Error("missing constructor")
	}
	if !strings.Contains(code, "foreign_keys") {
		t.Error("missing foreign_keys pragma")
	}
	if !strings.Contains(code, "func (s *SQLiteStore) ListVehicles(") {
		t.Error("missing ListVehicles implementation")
	}
}
