package envoy

import (
	"strings"
	"testing"

	"dual-era-mcp-gateway/internal/source"
)

func TestGenerateContainsCoreResources(t *testing.T) {
	cfg := Config{
		PublicPort:   8443,
		InternalPort: 10001,
		ModernHost:   "modern-facade",
		ModernPort:   8080,
		Sources: []source.SourceSpec{
			{Name: "weather", Type: source.TypeMCP, Endpoint: "http://weather:8080/mcp", Prefix: "weather"},
			{Name: "crm", Type: source.TypeREST, BaseURL: "https://crm.example.com", Prefix: "crm"},
		},
	}
	out, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, want := range []string{
		"name: public",
		"name: legacy_internal",
		`prefix: "/modern/mcp"`,
		`prefix: "/mcp"`,
		"envoy.filters.http.mcp_router",
		"envoy.clusters.mcp_multicluster",
		"modern_facade",
		"weather",
		"crm",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in generated config:\n%s", want, out)
		}
	}
}

func TestGenerateRejectsBadEndpoint(t *testing.T) {
	_, err := Generate(Config{Sources: []source.SourceSpec{{Name: "x", Type: source.TypeMCP, Endpoint: "://bad"}}})
	if err == nil {
		t.Fatal("expected error")
	}
}
