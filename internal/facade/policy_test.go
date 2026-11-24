package facade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/registry"
)

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func policyFacade(t *testing.T, up Upstream) *Facade {
	t.Helper()
	p, err := policy.New(policy.Config{
		Consumers: []policy.Consumer{
			{
				Name:    "acme",
				Tenant:  "acme",
				APIKeys: []policy.APIKey{{Name: "prod", SHA256: hashKey("acme-key")}},
				Allow: []policy.AllowRule{
					{Source: "weather", Tools: []string{"*"}},
					{Source: "crm", Tools: []string{"get_customer"}},
				},
			},
		},
		RateLimits: []policy.RateLimit{
			{Tenant: "acme", Source: "weather", Tool: "get_weather", RPM: 1},
		},
	})
	if err != nil {
		t.Fatalf("policy.New: %v", err)
	}
	f := newTestFacade(up)
	f.cfg.Policy = p
	return f
}

func TestPolicyUnauthenticated(t *testing.T) {
	f := policyFacade(t, &fakeUpstream{})
	rec := post(t, f, "tools/list", `{`+meta(mcp.Version20260728)+`}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPolicyForbidden(t *testing.T) {
	up := &fakeUpstream{result: json.RawMessage(`{"content":[]}`)}
	f := policyFacade(t, up)
	rec := post(t, f, "tools/call", `{"name":"crm__create_order","arguments":{},`+meta(mcp.Version20260728)+`}`, map[string]string{
		"Authorization": "Bearer acme-key",
		"Mcp-Name":      "crm__create_order",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPolicyRateLimited(t *testing.T) {
	up := &fakeUpstream{result: json.RawMessage(`{"content":[]}`)}
	f := policyFacade(t, up)
	headers := map[string]string{"Authorization": "Bearer acme-key", "Mcp-Name": "weather__get_weather"}
	body := `{"name":"weather__get_weather","arguments":{},` + meta(mcp.Version20260728) + `}`

	first := post(t, f, "tools/call", body, headers)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	second := post(t, f, "tools/call", body, headers)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d body=%s", second.Code, second.Body.String())
	}
	out := decode(t, second)
	errObj := out["error"].(map[string]any)
	if errObj["layer"] != "tool-rpm" {
		t.Fatalf("layer = %v", errObj["layer"])
	}
}

type fakeMetrics struct {
	auth, rate int
}

func (f *fakeMetrics) IncAuthDenied()  { f.auth++ }
func (f *fakeMetrics) IncRateLimited() { f.rate++ }

func TestPolicyMetrics(t *testing.T) {
	up := &fakeUpstream{result: json.RawMessage(`{"content":[]}`)}
	f := policyFacade(t, up)
	m := &fakeMetrics{}
	f.cfg.Metrics = m

	post(t, f, "tools/list", `{`+meta(mcp.Version20260728)+`}`, nil)
	if m.auth != 1 {
		t.Fatalf("auth = %d", m.auth)
	}
	headers := map[string]string{"Authorization": "Bearer acme-key", "Mcp-Name": "weather__get_weather"}
	body := `{"name":"weather__get_weather","arguments":{},` + meta(mcp.Version20260728) + `}`
	post(t, f, "tools/call", body, headers)
	post(t, f, "tools/call", body, headers)
	if m.rate != 1 {
		t.Fatalf("rate = %d", m.rate)
	}
}

func TestToolsListFilteredByPolicy(t *testing.T) {
	up := &fakeUpstream{tools: []registry.Tool{
		{Name: "weather__get_weather", OriginalName: "get_weather", Source: "weather"},
		{Name: "crm__create_order", OriginalName: "create_order", Source: "crm"},
	}}
	f := policyFacade(t, up)
	rec := post(t, f, "tools/list", `{`+meta(mcp.Version20260728)+`}`, map[string]string{"Authorization": "Bearer acme-key"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %+v", tools)
	}
	if tools[0].(map[string]any)["name"] != "weather__get_weather" {
		t.Fatalf("unexpected tool: %+v", tools[0])
	}
}
