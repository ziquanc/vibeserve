package cloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `tunnel: e53bc700-42c9-4592-b5db-4ca504fe391a
credentials-file: /Users/kent/.cloudflared/e53bc700-42c9-4592-b5db-4ca504fe391a.json

ingress:
  - service: http_status:404
`

func writeSampleConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadTunnelConfig(t *testing.T) {
	path := writeSampleConfig(t)
	cfg, err := LoadTunnelConfig(path)
	if err != nil {
		t.Fatalf("LoadTunnelConfig: %v", err)
	}
	if cfg.Tunnel != "e53bc700-42c9-4592-b5db-4ca504fe391a" {
		t.Errorf("Tunnel: got %q", cfg.Tunnel)
	}
	if len(cfg.Ingress) != 1 {
		t.Errorf("Ingress: got %d rules, want 1 (the catch-all)", len(cfg.Ingress))
	}
}

func TestAddIngress_NewHostname(t *testing.T) {
	path := writeSampleConfig(t)
	if err := AddIngress(path, "coffee.vibeserve.dev", "http://localhost:8080"); err != nil {
		t.Fatalf("AddIngress: %v", err)
	}
	cfg, _ := LoadTunnelConfig(path)
	if len(cfg.Ingress) != 2 {
		t.Fatalf("ingress count: got %d, want 2", len(cfg.Ingress))
	}
	if cfg.Ingress[0].Hostname != "coffee.vibeserve.dev" {
		t.Errorf("Ingress[0].Hostname: got %q", cfg.Ingress[0].Hostname)
	}
	if cfg.Ingress[0].Service != "http://localhost:8080" {
		t.Errorf("Ingress[0].Service: got %q", cfg.Ingress[0].Service)
	}
	last := cfg.Ingress[len(cfg.Ingress)-1]
	if last.Service != "http_status:404" || last.Hostname != "" {
		t.Errorf("catch-all displaced: last rule is %+v", last)
	}
}

func TestAddIngress_ReplacesExistingHostname(t *testing.T) {
	path := writeSampleConfig(t)
	_ = AddIngress(path, "coffee.vibeserve.dev", "http://localhost:8080")
	_ = AddIngress(path, "coffee.vibeserve.dev", "http://localhost:9090")
	cfg, _ := LoadTunnelConfig(path)
	if len(cfg.Ingress) != 2 {
		t.Fatalf("ingress count: got %d, want 2 (replace, not append)", len(cfg.Ingress))
	}
	if cfg.Ingress[0].Service != "http://localhost:9090" {
		t.Errorf("service not updated: got %q", cfg.Ingress[0].Service)
	}
}

func TestRemoveIngress(t *testing.T) {
	path := writeSampleConfig(t)
	_ = AddIngress(path, "coffee.vibeserve.dev", "http://localhost:8080")
	_ = AddIngress(path, "salon.vibeserve.dev", "http://localhost:8081")

	if err := RemoveIngress(path, "coffee.vibeserve.dev"); err != nil {
		t.Fatalf("RemoveIngress: %v", err)
	}
	cfg, _ := LoadTunnelConfig(path)
	if len(cfg.Ingress) != 2 {
		t.Fatalf("ingress count: got %d, want 2 (one removed, one kept, plus catch-all means 2)", len(cfg.Ingress))
	}
	for _, rule := range cfg.Ingress {
		if rule.Hostname == "coffee.vibeserve.dev" {
			t.Errorf("removed hostname still present: %+v", rule)
		}
	}
}

func TestRemoveIngress_Idempotent(t *testing.T) {
	path := writeSampleConfig(t)
	if err := RemoveIngress(path, "nonexistent.vibeserve.dev"); err != nil {
		t.Errorf("RemoveIngress on missing hostname should be a no-op, got: %v", err)
	}
}

func TestSaveTunnelConfig_PreservesYAMLShape(t *testing.T) {
	path := writeSampleConfig(t)
	_ = AddIngress(path, "coffee.vibeserve.dev", "http://localhost:8080")

	data, _ := os.ReadFile(path)
	out := string(data)
	if !strings.Contains(out, "tunnel: e53bc700") {
		t.Errorf("tunnel field missing after save: %s", out)
	}
	if !strings.Contains(out, "credentials-file:") {
		t.Errorf("credentials-file missing after save: %s", out)
	}
}
