package engine

import (
	"strings"
	"testing"
)

func TestGenerateCRUD_SoftDelete(t *testing.T) {
	routes, scripts := generateCRUD("pets", "/pets", "pet")

	// list should filter deleted
	for _, s := range scripts {
		switch s.Name {
		case "list_pets":
			if !strings.Contains(s.Code, "deleted_at IS NULL") {
				t.Error("list script should filter deleted_at IS NULL")
			}
		case "get_pet":
			if !strings.Contains(s.Code, "deleted_at IS NULL") {
				t.Error("get script should filter deleted_at IS NULL")
			}
		case "delete_pet":
			if strings.Contains(s.Code, "db.delete(") {
				t.Error("delete script should use soft delete, not db.delete()")
			}
			if !strings.Contains(s.Code, "deleted_at") {
				t.Error("delete script should set deleted_at")
			}
		}
	}
	_ = routes
}

func TestGenerateCustomRoute_SoftDelete_DELETE(t *testing.T) {
	_, script := generateCustomRoute("DELETE", "/pets/:id", "pets", nil)

	if strings.Contains(script.Code, "db.delete(") {
		t.Error("custom DELETE route should use soft delete")
	}
	if !strings.Contains(script.Code, "deleted_at") {
		t.Error("custom DELETE route should set deleted_at")
	}
}

func TestGenerateCustomRoute_SoftDelete_GET_List(t *testing.T) {
	_, script := generateCustomRoute("GET", "/pets", "pets", nil)

	if !strings.Contains(script.Code, "deleted_at IS NULL") {
		t.Error("custom GET list route should filter deleted_at IS NULL")
	}
}

func TestGenerateCustomRoute_SoftDelete_GET_ByID(t *testing.T) {
	_, script := generateCustomRoute("GET", "/pets/:id", "pets", nil)

	if !strings.Contains(script.Code, "deleted_at IS NULL") {
		t.Error("custom GET by ID route should filter deleted_at IS NULL")
	}
}
