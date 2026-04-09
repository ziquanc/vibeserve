package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGoType(t *testing.T) {
	tests := []struct {
		col  manifest.Column
		want string
	}{
		{manifest.Column{Type: "INTEGER", Primary: true, Auto: true}, "int64"},
		{manifest.Column{Type: "INTEGER"}, "int64"},
		{manifest.Column{Type: "TEXT"}, "string"},
		{manifest.Column{Type: "REAL"}, "float64"},
		{manifest.Column{Type: "BOOLEAN"}, "bool"},
		{manifest.Column{Type: "DATE"}, "time.Time"},
		{manifest.Column{Type: "DATETIME"}, "time.Time"},
	}
	for _, tt := range tests {
		got := GoType(tt.col)
		if got != tt.want {
			t.Errorf("GoType(%+v) = %q, want %q", tt.col, got, tt.want)
		}
	}
}

func TestGenerateModels(t *testing.T) {
	schemas := []manifest.Schema{
		{
			Table: "vehicles",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "make", Type: "TEXT", Required: true},
				{Name: "plate", Type: "TEXT", Unique: true},
				{Name: "daily_rate", Type: "REAL", Required: true},
				{Name: "available", Type: "BOOLEAN", Default: true},
			},
		},
	}

	code := GenerateModels(schemas)

	if !strings.Contains(code, "package model") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "type Vehicle struct") {
		t.Error("missing Vehicle struct")
	}
	if !strings.Contains(code, "ID") {
		t.Error("missing ID field")
	}
	if !strings.Contains(code, `db:"id"`) {
		t.Error("missing db tag for id")
	}
	if !strings.Contains(code, `json:"id"`) {
		t.Error("missing json tag for id")
	}
	if !strings.Contains(code, "float64") {
		t.Error("missing float64 for REAL column")
	}
	if !strings.Contains(code, "bool") {
		t.Error("missing bool for BOOLEAN column")
	}

	schemasWithDate := []manifest.Schema{
		{
			Table: "bookings",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "start_date", Type: "DATE", Required: true},
			},
		},
	}
	codeWithDate := GenerateModels(schemasWithDate)
	if !strings.Contains(codeWithDate, `"time"`) {
		t.Error("missing time import for DATE column")
	}
}
