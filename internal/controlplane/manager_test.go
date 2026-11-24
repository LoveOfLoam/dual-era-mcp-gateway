package controlplane

import (
	"testing"

	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/source"
)

func baseConfig() Config {
	return Config{
		Sources: []source.SourceSpec{
			{
				Name:     "weather",
				Type:     source.TypeMCP,
				Endpoint: "http://weather:8080/mcp",
				Prefix:   "weather",
				Tools:    []source.ToolSpec{{Name: "get_weather"}},
			},
			{
				Name:    "crm",
				Type:    source.TypeREST,
				BaseURL: "https://crm.example.com",
				Prefix:  "crm",
				Tools:   []source.ToolSpec{{Name: "get_customer"}},
			},
		},
		Consumers: []policy.Consumer{
			{
				Name:    "acme",
				Tenant:  "acme",
				APIKeys: []policy.APIKey{{Name: "prod", SHA256: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}},
				Allow:   []policy.AllowRule{{Source: "weather", Tools: []string{"*"}}},
			},
		},
		RateLimits: []policy.RateLimit{
			{Tenant: "acme", Source: "weather", Tool: "get_weather", RPM: 10},
		},
	}
}

func TestNewAndVersion(t *testing.T) {
	m, err := New(baseConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cur := m.Current()
	if cur.Version == "" {
		t.Fatal("empty version")
	}
	if m2, _ := New(baseConfig()); m2.Current().Version != cur.Version {
		t.Fatal("version is not deterministic")
	}
}

func TestRejectUnknownSourceRef(t *testing.T) {
	cfg := baseConfig()
	cfg.RateLimits[0].Source = "missing"
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectUnknownToolRef(t *testing.T) {
	cfg := baseConfig()
	cfg.RateLimits[0].Tool = "missing"
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyAndRollback(t *testing.T) {
	m, err := New(baseConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	v1 := m.Current().Version

	cfg2 := baseConfig()
	cfg2.Sources[0].Endpoint = "http://weather2:8080/mcp"
	snap2, err := m.Apply(cfg2)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if snap2.Version == v1 {
		t.Fatal("version did not change")
	}
	if len(m.History()) != 2 {
		t.Fatalf("history = %d", len(m.History()))
	}
	restored, err := m.Rollback(v1)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if restored.Version != v1 || m.Current().Version != v1 {
		t.Fatalf("restored = %s current = %s", restored.Version, m.Current().Version)
	}
}

func TestRollbackUnknownVersion(t *testing.T) {
	m, _ := New(baseConfig())
	if _, err := m.Rollback("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyInvalidKeepsCurrent(t *testing.T) {
	m, _ := New(baseConfig())
	before := m.Current().Version
	bad := baseConfig()
	bad.Sources[0].Endpoint = ""
	if _, err := m.Apply(bad); err == nil {
		t.Fatal("expected error")
	}
	if m.Current().Version != before {
		t.Fatal("current changed after invalid apply")
	}
}
