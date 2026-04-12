package manifest

import "testing"

func TestInferForeignKeys(t *testing.T) {
	schemas := []Schema{
		{Table: "users", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT"},
		}},
		{Table: "posts", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "user_id", Type: "INTEGER"},
			{Name: "title", Type: "TEXT"},
		}},
		{Table: "comments", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "post_id", Type: "INTEGER"},
			{Name: "user_id", Type: "INTEGER"},
			{Name: "body", Type: "TEXT"},
		}},
	}

	result := InferForeignKeys(schemas)

	// posts.user_id should reference users.id
	for _, col := range result[1].Columns {
		if col.Name == "user_id" && col.References != "users.id" {
			t.Errorf("posts.user_id should reference users.id, got %q", col.References)
		}
	}

	// comments.post_id should reference posts.id
	// comments.user_id should reference users.id
	for _, col := range result[2].Columns {
		if col.Name == "post_id" && col.References != "posts.id" {
			t.Errorf("comments.post_id should reference posts.id, got %q", col.References)
		}
		if col.Name == "user_id" && col.References != "users.id" {
			t.Errorf("comments.user_id should reference users.id, got %q", col.References)
		}
	}
}

func TestInferForeignKeys_SkipsExisting(t *testing.T) {
	schemas := []Schema{
		{Table: "users", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
		}},
		{Table: "posts", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "user_id", Type: "INTEGER", References: "users.id"},
		}},
	}

	result := InferForeignKeys(schemas)

	// Should not modify existing reference
	if result[1].Columns[1].References != "users.id" {
		t.Error("should not modify existing FK reference")
	}
}

func TestInferForeignKeys_NoMatch(t *testing.T) {
	schemas := []Schema{
		{Table: "posts", Columns: []Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "author_id", Type: "INTEGER"}, // no "authors" table
		}},
	}

	result := InferForeignKeys(schemas)

	if result[0].Columns[1].References != "" {
		t.Error("should not add FK when referenced table doesn't exist")
	}
}
