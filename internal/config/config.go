package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the .vibe/config.yaml file.
type Config struct {
	Provider   string       `yaml:"provider"`
	APIKeyEnv  string       `yaml:"api_key_env,omitempty"`
	APIKeyVal  string       `yaml:"api_key,omitempty"` // direct key storage (used if api_key_env is empty or env var unset)
	Model      string       `yaml:"model"`
	OllamaHost string       `yaml:"ollama_host,omitempty"`
	BaseURL    string       `yaml:"base_url,omitempty"` // OpenAI-compatible endpoint URL
	MaxTokens  int          `yaml:"max_tokens,omitempty"`
	Server     ServerConfig `yaml:"server"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
	CORS bool   `yaml:"cors"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Provider:   "claude",
		APIKeyEnv:  "ANTHROPIC_API_KEY",
		Model:      "claude-sonnet-4-6-20250514",
		OllamaHost: "http://localhost:11434",
		Server: ServerConfig{
			Port: 8080,
			Host: "localhost",
			CORS: true,
		},
	}
}

// Load reads and parses a YAML config file from the given path.
// If the file does not exist, returns DefaultConfig with no error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// Save writes the config to a YAML file at the given path.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// APIKey reads the API key. Checks the environment variable first (APIKeyEnv),
// falls back to the direct value (APIKeyVal) stored in config.
func (c *Config) APIKey() string {
	if c.APIKeyEnv != "" {
		if v := os.Getenv(c.APIKeyEnv); v != "" {
			return v
		}
	}
	return c.APIKeyVal
}

// Validate checks that the config has all required fields.
func (c *Config) Validate() error {
	switch c.Provider {
	case "claude":
		if c.APIKey() == "" {
			return fmt.Errorf("provider 'claude' requires %s environment variable to be set", c.APIKeyEnv)
		}
	case "ollama":
		if c.OllamaHost == "" {
			return fmt.Errorf("provider 'ollama' requires ollama_host to be set")
		}
	case "openai":
		if c.APIKey() == "" {
			return fmt.Errorf("provider 'openai' requires %s environment variable to be set", c.APIKeyEnv)
		}
		if c.BaseURL == "" {
			return fmt.Errorf("provider 'openai' requires base_url to be set")
		}
	default:
		return fmt.Errorf("unknown provider %q (supported: claude, openai, ollama)", c.Provider)
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	return nil
}
