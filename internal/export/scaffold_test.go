package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateMain(t *testing.T) {
	routes := []manifest.Route{
		{Path: "/vehicles", Method: "GET", Script: "list_vehicles"},
		{Path: "/vehicles/:id", Method: "GET", Script: "get_vehicle"},
		{Path: "/bookings", Method: "POST", Script: "create_booking"},
	}

	code := GenerateMain("my-api", routes)

	if !strings.Contains(code, "package main") {
		t.Error("missing package main")
	}
	if !strings.Contains(code, "chi.NewRouter") {
		t.Error("missing chi router creation")
	}
	if !strings.Contains(code, "middleware.RequestID") {
		t.Error("missing RequestID middleware")
	}
	if !strings.Contains(code, "middleware.RealIP") {
		t.Error("missing RealIP middleware")
	}
	if !strings.Contains(code, "middleware.Logger") {
		t.Error("missing Logger middleware")
	}
	if !strings.Contains(code, "middleware.Recoverer") {
		t.Error("missing Recoverer middleware")
	}
	if !strings.Contains(code, "SIGINT") || !strings.Contains(code, "SIGTERM") {
		t.Error("missing graceful shutdown signals")
	}
	if !strings.Contains(code, "r.Get(\"/vehicles\"") {
		t.Error("missing GET /vehicles route")
	}
	if !strings.Contains(code, "r.Get(\"/vehicles/{id}\"") {
		t.Error("missing GET /vehicles/{id} route")
	}
	if !strings.Contains(code, "r.Post(\"/bookings\"") {
		t.Error("missing POST /bookings route")
	}
}

func TestGenerateGoMod(t *testing.T) {
	code := GenerateGoMod("my-api")

	if !strings.Contains(code, "module my-api") {
		t.Error("missing module declaration")
	}
	if !strings.Contains(code, "go-chi/chi") {
		t.Error("missing chi dependency")
	}
	if !strings.Contains(code, "jmoiron/sqlx") {
		t.Error("missing sqlx dependency")
	}
	if !strings.Contains(code, "modernc.org/sqlite") {
		t.Error("missing sqlite dependency")
	}
}

func TestGenerateDockerfile(t *testing.T) {
	code := GenerateDockerfile("my-api")

	if !strings.Contains(code, "FROM golang:") {
		t.Error("missing Go builder stage")
	}
	if !strings.Contains(code, "CGO_ENABLED=0") {
		t.Error("missing CGO_ENABLED=0")
	}
	if !strings.Contains(code, "EXPOSE") {
		t.Error("missing EXPOSE")
	}
}

func TestGenerateREADME(t *testing.T) {
	m := &manifest.Manifest{
		Name:        "Car Rental API",
		Description: "Rent cars in Malaysia",
		Schemas: []manifest.Schema{
			{Table: "vehicles", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER"},
				{Name: "make", Type: "TEXT"},
			}},
		},
		Routes: []manifest.Route{
			{Path: "/vehicles", Method: "GET", Description: "List all vehicles"},
		},
	}

	readme := GenerateREADME(m)

	if !strings.Contains(readme, "Car Rental API") {
		t.Error("missing project name")
	}
	if !strings.Contains(readme, "go run ./cmd/api") {
		t.Error("missing run instructions")
	}
	if !strings.Contains(readme, "GET") {
		t.Error("missing route table")
	}
	if !strings.Contains(readme, "vehicles") {
		t.Error("missing schema documentation")
	}
}
