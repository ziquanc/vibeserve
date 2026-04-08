package llm

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestBuildSystemPrompt_NilManifest(t *testing.T) {
	prompt := BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "VibeServe") {
		t.Error("expected prompt to mention VibeServe")
	}
	if !strings.Contains(prompt, "response.fail") {
		t.Error("expected prompt to mention response.fail")
	}
	if !strings.Contains(prompt, "log.err") {
		t.Error("expected prompt to mention log.err")
	}
	if !strings.Contains(prompt, "Migration Memory Rule") {
		t.Error("expected prompt to contain Migration Memory Rule")
	}
	if !strings.Contains(prompt, "No existing manifest") {
		t.Error("expected prompt to say no existing manifest")
	}
	if !strings.Contains(prompt, "ONLY valid JSON") {
		t.Error("expected prompt to instruct JSON-only output")
	}
}

func TestBuildSystemPrompt_WithManifest(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "test-api",
		Schemas: []manifest.Schema{
			{Table: "users", Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			}},
		},
	}

	prompt := BuildSystemPrompt(m)

	if !strings.Contains(prompt, "test-api") {
		t.Error("expected prompt to contain current manifest name")
	}
	if !strings.Contains(prompt, "Current Manifest") {
		t.Error("expected prompt to have Current Manifest section")
	}
	if !strings.Contains(prompt, `"users"`) {
		t.Error("expected prompt to contain table name from manifest")
	}
}

func TestBuildSystemPrompt_ContainsAllStdlib(t *testing.T) {
	prompt := BuildSystemPrompt(nil)

	required := []string{
		"db.query", "db.query_one", "db.insert", "db.update", "db.delete", "db.count",
		"request.param", "request.query", "request.body", "request.header", "request.method", "request.auth",
		"response.json", "response.fail", "response.header", "response.redirect",
		"date.now", "date.diff_days", "date.add_days", "date.format",
		"crypto.hash", "crypto.uuid", "crypto.random",
		"log.info", "log.warn", "log.err",
	}

	for _, fn := range required {
		if !strings.Contains(prompt, fn) {
			t.Errorf("system prompt missing stdlib function: %s", fn)
		}
	}
}

func TestBuildSystemPrompt_TengoKeywordWarnings(t *testing.T) {
	prompt := BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "\"error\" is a reserved keyword in Tengo") {
		t.Error("expected Tengo keyword warning")
	}
	if strings.Contains(prompt, "response.error(") {
		t.Error("prompt should NOT contain response.error()")
	}
	if strings.Contains(prompt, "log.error(") {
		t.Error("prompt should NOT contain log.error()")
	}
}

func TestExtractJSON_PlainJSON(t *testing.T) {
	raw := `{"version": "1.0", "name": "test"}`
	result, err := ExtractJSON(raw)
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if result != raw {
		t.Errorf("expected %q, got %q", raw, result)
	}
}

func TestExtractJSON_MarkdownFences(t *testing.T) {
	raw := "Here is the manifest:\n```json\n{\"version\": \"1.0\", \"name\": \"test\"}\n```\nDone!"
	result, err := ExtractJSON(raw)
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if result != `{"version": "1.0", "name": "test"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSON_SurroundingText(t *testing.T) {
	raw := "Sure, here is the manifest: {\"version\": \"1.0\"} I hope this helps!"
	result, err := ExtractJSON(raw)
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if result != `{"version": "1.0"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	raw := "I don't know how to help with that."
	_, err := ExtractJSON(raw)
	if err == nil {
		t.Error("expected error for no JSON")
	}
}

func TestParseManifestResponse_Valid(t *testing.T) {
	raw := `{"version": "1.0", "name": "test", "description": "A test", "schemas": [], "routes": [], "scripts": [], "seeds": []}`
	m, err := ParseManifestResponse(raw)
	if err != nil {
		t.Fatalf("ParseManifestResponse: %v", err)
	}
	if m.Name != "test" {
		t.Errorf("expected name 'test', got %q", m.Name)
	}
	if m.Version != "1.0" {
		t.Errorf("expected version '1.0', got %q", m.Version)
	}
}

func TestParseManifestResponse_WithFences(t *testing.T) {
	raw := "```json\n{\"version\": \"1.0\", \"name\": \"fenced\"}\n```"
	m, err := ParseManifestResponse(raw)
	if err != nil {
		t.Fatalf("ParseManifestResponse: %v", err)
	}
	if m.Name != "fenced" {
		t.Errorf("expected name 'fenced', got %q", m.Name)
	}
}

func TestParseManifestResponse_Invalid(t *testing.T) {
	raw := "not json at all"
	_, err := ParseManifestResponse(raw)
	if err == nil {
		t.Error("expected error for invalid response")
	}
}
