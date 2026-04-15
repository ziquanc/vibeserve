package cloud

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TunnelConfig represents the subset of ~/.cloudflared/config.yml we care about.
// Unknown fields are preserved via inline map so hand-edits to other settings
// don't get clobbered.
type TunnelConfig struct {
	Tunnel          string                 `yaml:"tunnel"`
	CredentialsFile string                 `yaml:"credentials-file"`
	Ingress         []IngressRule          `yaml:"ingress"`
	Extra           map[string]interface{} `yaml:",inline"`
}

// IngressRule maps a hostname to a backend service. The catch-all rule
// has Hostname == "" and Service == "http_status:404".
type IngressRule struct {
	Hostname string `yaml:"hostname,omitempty"`
	Service  string `yaml:"service"`
}

// LoadTunnelConfig reads and parses a cloudflared YAML config file.
func LoadTunnelConfig(path string) (*TunnelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tunnel config: %w", err)
	}
	var cfg TunnelConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse tunnel config: %w", err)
	}
	return &cfg, nil
}

// SaveTunnelConfig serializes cfg back to the given path with 0o644 perms.
func SaveTunnelConfig(path string, cfg *TunnelConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal tunnel config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write tunnel config: %w", err)
	}
	return nil
}

// AddIngress upserts a hostname -> service mapping. New rules are inserted
// BEFORE the catch-all (last rule with empty hostname). If the hostname
// already exists, its service is updated in place.
func AddIngress(path, hostname, service string) error {
	cfg, err := LoadTunnelConfig(path)
	if err != nil {
		return err
	}

	for i, rule := range cfg.Ingress {
		if rule.Hostname == hostname {
			cfg.Ingress[i].Service = service
			return SaveTunnelConfig(path, cfg)
		}
	}

	insertAt := len(cfg.Ingress)
	for i, rule := range cfg.Ingress {
		if rule.Hostname == "" {
			insertAt = i
			break
		}
	}
	newRule := IngressRule{Hostname: hostname, Service: service}
	cfg.Ingress = append(cfg.Ingress[:insertAt], append([]IngressRule{newRule}, cfg.Ingress[insertAt:]...)...)

	return SaveTunnelConfig(path, cfg)
}

// RemoveIngress deletes the rule for the given hostname. No-op if absent.
func RemoveIngress(path, hostname string) error {
	cfg, err := LoadTunnelConfig(path)
	if err != nil {
		return err
	}
	filtered := make([]IngressRule, 0, len(cfg.Ingress))
	for _, rule := range cfg.Ingress {
		if rule.Hostname == hostname {
			continue
		}
		filtered = append(filtered, rule)
	}
	if len(filtered) == len(cfg.Ingress) {
		return nil
	}
	cfg.Ingress = filtered
	return SaveTunnelConfig(path, cfg)
}
