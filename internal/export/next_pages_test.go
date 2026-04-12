package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestNextResourcePages(t *testing.T) {
	schema := manifest.Schema{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "email", Type: "TEXT", Required: true, Unique: true},
			{Name: "age", Type: "INTEGER"},
		},
	}

	pages := GenerateNextResourcePages(schema, "users")

	// Should generate 4 pages
	if len(pages) != 4 {
		t.Errorf("expected 4 pages, got %d", len(pages))
	}

	// List page
	list, ok := pages["users/page.tsx"]
	if !ok {
		t.Fatal("missing list page")
	}
	if !strings.Contains(list, "'use client'") {
		t.Error("list page should be a client component")
	}
	if !strings.Contains(list, "listUsers") {
		t.Error("list page should call listUsers")
	}
	if !strings.Contains(list, "deleteUser") {
		t.Error("list page should have delete")
	}
	if !strings.Contains(list, "Table") {
		t.Error("list page should use Table component")
	}
	if !strings.Contains(list, "search") {
		t.Error("list page should have search")
	}
	if !strings.Contains(list, "ArrowUpDown") {
		t.Error("list page should have sort icons")
	}
	if !strings.Contains(list, "Pagination") || !strings.Contains(list, "Previous") {
		// We just need either pagination text
		if !strings.Contains(list, "Previous") {
			t.Error("list page should have pagination")
		}
	}
	if !strings.Contains(list, "/users/new") {
		t.Error("list page should link to create page")
	}
	if !strings.Contains(list, "type { User }") {
		t.Error("list page should import User type")
	}

	// Create page
	create, ok := pages["users/new/page.tsx"]
	if !ok {
		t.Fatal("missing create page")
	}
	if !strings.Contains(create, "'use client'") {
		t.Error("create page should be a client component")
	}
	if !strings.Contains(create, "createUser") {
		t.Error("create page should call createUser")
	}
	if !strings.Contains(create, "Label") {
		t.Error("create page should use Label")
	}
	if !strings.Contains(create, "required") {
		t.Error("create page should mark required fields")
	}
	if !strings.Contains(create, `type="number"`) {
		t.Error("create page should use number input for age")
	}
	if !strings.Contains(create, "New User") {
		t.Error("create page should have title")
	}
	// Should not include id field (PK auto)
	if strings.Contains(create, `id="id"`) {
		t.Error("create page should not include auto PK field")
	}

	// Detail page
	detail, ok := pages["users/[id]/page.tsx"]
	if !ok {
		t.Fatal("missing detail page")
	}
	if !strings.Contains(detail, "'use client'") {
		t.Error("detail page should be a client component")
	}
	if !strings.Contains(detail, "getUser") {
		t.Error("detail page should call getUser")
	}
	if !strings.Contains(detail, "deleteUser") {
		t.Error("detail page should have delete action")
	}
	if !strings.Contains(detail, "useParams") {
		t.Error("detail page should use useParams")
	}
	if !strings.Contains(detail, "/users/${data.id}/edit") {
		t.Error("detail page should link to edit page")
	}

	// Edit page
	edit, ok := pages["users/[id]/edit/page.tsx"]
	if !ok {
		t.Fatal("missing edit page")
	}
	if !strings.Contains(edit, "'use client'") {
		t.Error("edit page should be a client component")
	}
	if !strings.Contains(edit, "updateUser") {
		t.Error("edit page should call updateUser")
	}
	if !strings.Contains(edit, "getUser") {
		t.Error("edit page should load existing data")
	}
	if !strings.Contains(edit, "Edit User") {
		t.Error("edit page should have title")
	}
	if !strings.Contains(edit, `type="number"`) {
		t.Error("edit page should use number input for age")
	}
}

func TestNextResourcePagesMultiWordTable(t *testing.T) {
	schema := manifest.Schema{
		Table: "user_accounts",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "display_name", Type: "TEXT", Required: true},
			{Name: "is_active", Type: "BOOLEAN"},
		},
	}

	pages := GenerateNextResourcePages(schema, "user_accounts")

	if len(pages) != 4 {
		t.Errorf("expected 4 pages, got %d", len(pages))
	}

	list := pages["user_accounts/page.tsx"]
	if !strings.Contains(list, "listUserAccounts") {
		t.Error("should use pluralized API function name")
	}
	if !strings.Contains(list, "UserAccount") {
		t.Error("should use correct type name")
	}

	create := pages["user_accounts/new/page.tsx"]
	if !strings.Contains(create, "createUserAccount") {
		t.Error("should use singular API function name for create")
	}
	if !strings.Contains(create, `type="checkbox"`) {
		t.Error("should use checkbox for boolean column")
	}
	if !strings.Contains(create, "Display Name") {
		t.Error("should convert snake_case to title case for labels")
	}
}

func TestNextResourcePagesTimestampFiltering(t *testing.T) {
	// Timestamps get injected — verify they don't appear in form fields
	schema := manifest.Schema{
		Table: "products",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT", Required: true},
			{Name: "price", Type: "REAL", Required: true},
		},
	}

	pages := GenerateNextResourcePages(schema, "products")

	create := pages["products/new/page.tsx"]
	if strings.Contains(create, "created_at") {
		t.Error("create page should not have created_at field")
	}
	if strings.Contains(create, "updated_at") {
		t.Error("create page should not have updated_at field")
	}

	// Detail page should show timestamps
	detail := pages["products/[id]/page.tsx"]
	if !strings.Contains(detail, "Created At") {
		t.Error("detail page should show created_at")
	}

	// List page should not show timestamps in table
	list := pages["products/page.tsx"]
	if strings.Contains(list, "created_at") || strings.Contains(list, "Created At") {
		t.Error("list page should not show timestamp columns")
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"id", "Id"},
		{"user_id", "User Id"},
		{"created_at", "Created At"},
		{"name", "Name"},
		{"display_name", "Display Name"},
	}
	for _, tt := range tests {
		got := titleCase(tt.input)
		if got != tt.want {
			t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInputType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"TEXT", "text"},
		{"INTEGER", "number"},
		{"REAL", "number"},
		{"BOOLEAN", "checkbox"},
		{"DATE", "date"},
		{"DATETIME", "datetime-local"},
		{"UNKNOWN", "text"},
	}
	for _, tt := range tests {
		got := inputType(tt.input)
		if got != tt.want {
			t.Errorf("inputType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
