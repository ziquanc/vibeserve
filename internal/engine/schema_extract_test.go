package engine

import (
	"testing"
)

func TestExtractSchemasFromSteps(t *testing.T) {
	steps := []string{
		"Create core tables: users (id, name, email, role, exam_type), subjects (id, name, code, exam_type: SPM/STPM/UEC), topics (id, subject_id FK, name, level, description)",
		"Create question bank: questions (id, topic_id FK, question_text, question_type: mcq/fill_blank, difficulty: easy/medium/hard, options, correct_answer, explanation)",
		"Create test tables: test_attempts (id, user_id FK, score, status: in_progress/completed, started_at, completed_at), attempt_answers (id, attempt_id FK, question_id FK, selected_option, is_correct)",
		"Create analytics: topic_mastery (id, user_id FK, topic_id FK, mastery_pct, questions_attempted, questions_correct)",
		"Seed data",
	}

	schemas := extractSchemasFromSteps(steps)

	if len(schemas) < 5 {
		t.Errorf("expected at least 5 tables, got %d", len(schemas))
		for _, s := range schemas {
			t.Logf("  table: %s (%d cols)", s.Table, len(s.Columns))
		}
	}

	// Check that users table was extracted
	found := false
	for _, s := range schemas {
		if s.Table == "users" {
			found = true
			if len(s.Columns) < 3 {
				t.Errorf("users should have at least 3 columns, got %d", len(s.Columns))
			}
			// Check id is PK
			for _, c := range s.Columns {
				if c.Name == "id" && !c.Primary {
					t.Error("id should be primary key")
				}
			}
		}
	}
	if !found {
		t.Error("should extract users table")
	}

	// Check FK detection
	for _, s := range schemas {
		if s.Table == "topics" {
			for _, c := range s.Columns {
				if c.Name == "subject_id" && c.References == "" {
					t.Error("subject_id should have FK reference")
				}
			}
		}
	}
}

func TestExtractSchemasFromSteps_Empty(t *testing.T) {
	schemas := extractSchemasFromSteps(nil)
	if len(schemas) != 0 {
		t.Errorf("expected 0 schemas from nil steps, got %d", len(schemas))
	}
}

func TestExtractSchemasFromSteps_NoTables(t *testing.T) {
	steps := []string{
		"Implement routes for user management",
		"Add seed data",
	}
	schemas := extractSchemasFromSteps(steps)
	// May or may not extract anything — just shouldn't panic
	_ = schemas
}
