package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != "claude" {
		t.Errorf("expected provider 'claude', got %q", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-6-20250514', got %q", cfg.Model)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", cfg.Server.Host)
	}
	if !cfg.Server.CORS {
		t.Error("expected CORS true by default")
	}
	if cfg.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("expected api_key_env 'ANTHROPIC_API_KEY', got %q", cfg.APIKeyEnv)
	}
	if cfg.OllamaHost != "http://localhost:11434" {
		t.Errorf("expected ollama_host 'http://localhost:11434', got %q", cfg.OllamaHost)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load nonexistent: %v", err)
	}
	if cfg.Provider != "claude" {
		t.Errorf("expected default provider 'claude', got %q", cfg.Provider)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	yaml := `provider: ollama
api_key_env: MY_KEY
model: llama3
ollama_host: http://myhost:11434
server:
  port: 9090
  host: 0.0.0.0
  cors: false
`
	os.WriteFile(path, []byte(yaml), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "ollama" {
		t.Errorf("expected 'ollama', got %q", cfg.Provider)
	}
	if cfg.APIKeyEnv != "MY_KEY" {
		t.Errorf("expected 'MY_KEY', got %q", cfg.APIKeyEnv)
	}
	if cfg.Model != "llama3" {
		t.Errorf("expected 'llama3', got %q", cfg.Model)
	}
	if cfg.OllamaHost != "http://myhost:11434" {
		t.Errorf("expected 'http://myhost:11434', got %q", cfg.OllamaHost)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got %q", cfg.Server.Host)
	}
	if cfg.Server.CORS {
		t.Error("expected CORS false")
	}
}

func TestLoad_PartialYAML_DefaultsFill(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	yaml := `provider: ollama
model: codellama
`
	os.WriteFile(path, []byte(yaml), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "ollama" {
		t.Errorf("expected 'ollama', got %q", cfg.Provider)
	}
	if cfg.Model != "codellama" {
		t.Errorf("expected 'codellama', got %q", cfg.Model)
	}
	// Defaults should still apply
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.OllamaHost != "http://localhost:11434" {
		t.Errorf("expected default ollama_host, got %q", cfg.OllamaHost)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte("{invalid: yaml: [unclosed"), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestAPIKey(t *testing.T) {
	cfg := &Config{APIKeyEnv: "TEST_VIBESERVE_KEY_12345"}
	os.Setenv("TEST_VIBESERVE_KEY_12345", "sk-secret-123")
	defer os.Unsetenv("TEST_VIBESERVE_KEY_12345")

	key := cfg.APIKey()
	if key != "sk-secret-123" {
		t.Errorf("expected 'sk-secret-123', got %q", key)
	}
}

func TestAPIKey_NotSet(t *testing.T) {
	cfg := &Config{APIKeyEnv: "NONEXISTENT_VAR_VIBESERVE_TEST"}
	os.Unsetenv("NONEXISTENT_VAR_VIBESERVE_TEST")

	key := cfg.APIKey()
	if key != "" {
		t.Errorf("expected empty string, got %q", key)
	}
}

func TestValidate_Claude_NoKey(t *testing.T) {
	cfg := &Config{
		Provider:  "claude",
		APIKeyEnv: "NONEXISTENT_VAR_VIBESERVE_TEST",
		Server:    ServerConfig{Port: 8080},
	}
	os.Unsetenv("NONEXISTENT_VAR_VIBESERVE_TEST")

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error when Claude API key is missing")
	}
}

func TestValidate_Ollama_OK(t *testing.T) {
	cfg := &Config{
		Provider:   "ollama",
		OllamaHost: "http://localhost:11434",
		Server:     ServerConfig{Port: 8080},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error for valid ollama config, got: %v", err)
	}
}

func TestValidate_UnknownProvider(t *testing.T) {
	cfg := &Config{
		Provider: "gpt",
		Server:   ServerConfig{Port: 8080},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Provider:   "ollama",
		OllamaHost: "http://localhost:11434",
		Server:     ServerConfig{Port: 0},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid port")
	}
}
