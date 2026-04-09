package engine

import (
	"testing"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestScoreHeuristics_PureCRUD(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/items", Method: "GET", Script: "list_items"},
			{Path: "/items/:id", Method: "GET", Script: "get_item"},
			{Path: "/items", Method: "POST", Script: "create_item"},
		},
		Scripts: []manifest.Script{
			{Name: "list_items", Code: "result := db.query(\"SELECT * FROM items\", [])\nresponse.json(result)"},
			{Name: "get_item", Code: "id := request.param(\"id\")\nrow := db.query_one(\"SELECT * FROM items WHERE id = ?\", [id])\nresponse.json(row)"},
			{Name: "create_item", Code: "body := request.body()\nresult := db.insert(\"items\", body)\nresponse.json(result, 201)"},
		},
	}
	result := ScoreHeuristics(m)
	if result.Score > 3 {
		t.Errorf("pure CRUD should score low, got %d", result.Score)
	}
	if len(result.Suggestions) == 0 {
		t.Error("pure CRUD should have suggestions")
	}
}

func TestScoreHeuristics_StateTransition(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/exams/:id/start", Method: "POST", Script: "start_exam"},
			{Path: "/exams/:id/submit", Method: "PATCH", Script: "submit_exam"},
		},
		Scripts: []manifest.Script{
			{Name: "start_exam", Code: "db.update(\"exams\", id, {status: \"in_progress\"})"},
			{Name: "submit_exam", Code: "db.update(\"exams\", id, {status: \"completed\"})"},
		},
	}
	result := ScoreHeuristics(m)
	if result.Score < 2 {
		t.Errorf("state transitions should score >= 2, got %d", result.Score)
	}
	if len(result.Hints) == 0 {
		t.Error("should have hints about state transitions")
	}
}

func TestScoreHeuristics_ComputedAggregation(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/students/:id/progress", Method: "GET", Script: "student_progress"},
		},
		Scripts: []manifest.Script{
			{Name: "student_progress", Code: "result := db.query(\"SELECT AVG(score) as avg_score, COUNT(*) as total FROM submissions WHERE student_id = ?\", [id])\nresponse.json(result)"},
		},
	}
	result := ScoreHeuristics(m)
	if result.Score < 2 {
		t.Errorf("aggregation should score >= 2, got %d", result.Score)
	}
}

func TestScoreHeuristics_ValidationGuard(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/exams/:id/submit", Method: "POST", Script: "submit_exam"},
		},
		Scripts: []manifest.Script{
			{Name: "submit_exam", Code: "exam := db.query_one(\"SELECT * FROM exams WHERE id = ?\", [id])\nif exam.status != \"in_progress\" {\n  response.fail(400, \"Exam not in progress\")\n}\ndb.update(\"exams\", id, {status: \"submitted\"})"},
		},
	}
	result := ScoreHeuristics(m)
	if result.Score < 1 {
		t.Errorf("validation guard should score >= 1, got %d", result.Score)
	}
}

func TestScoreHeuristics_LifecycleHook(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/submissions", Method: "POST", Script: "submit_answer"},
		},
		Scripts: []manifest.Script{
			{Name: "submit_answer", Code: "body := request.body()\nresult := db.insert(\"submissions\", body)\ndb.update(\"students\", body.student_id, {last_submission: date.now()})\nresponse.json(result, 201)"},
		},
	}
	result := ScoreHeuristics(m)
	if result.Score < 1 {
		t.Errorf("lifecycle hook should score >= 1, got %d", result.Score)
	}
}

func TestScoreHeuristics_CapsAt10(t *testing.T) {
	m := &manifest.Manifest{
		Routes: []manifest.Route{
			{Path: "/a/:id/start", Method: "POST", Script: "s1"},
			{Path: "/a/:id/stop", Method: "POST", Script: "s2"},
			{Path: "/b/:id/activate", Method: "POST", Script: "s3"},
			{Path: "/b/:id/deactivate", Method: "POST", Script: "s4"},
			{Path: "/c/:id/approve", Method: "PATCH", Script: "s5"},
			{Path: "/stats", Method: "GET", Script: "s6"},
		},
		Scripts: []manifest.Script{
			{Name: "s1", Code: "db.update(\"a\", id, {})"},
			{Name: "s2", Code: "db.update(\"a\", id, {})"},
			{Name: "s3", Code: "db.update(\"b\", id, {})"},
			{Name: "s4", Code: "db.update(\"b\", id, {})"},
			{Name: "s5", Code: "response.fail(400, \"nope\")\ndb.update(\"c\", id, {})"},
			{Name: "s6", Code: "db.query(\"SELECT COUNT(*) FROM x\", [])"},
		},
	}
	result := ScoreHeuristics(m)
	if result.Score > 10 {
		t.Errorf("score should cap at 10, got %d", result.Score)
	}
}
