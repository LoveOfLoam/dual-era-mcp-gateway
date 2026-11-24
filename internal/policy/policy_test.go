package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New(Config{
		Consumers: []Consumer{
			{
				Name:   "acme",
				Tenant: "acme",
				APIKeys: []APIKey{
					{Name: "prod", SHA256: hash("acme-key")},
				},
				Allow: []AllowRule{
					{Source: "weather", Tools: []string{"*"}},
					{Source: "crm", Tools: []string{"get_customer"}},
				},
			},
			{
				Name:   "beta",
				Tenant: "beta",
				APIKeys: []APIKey{
					{Name: "prod", SHA256: hash("beta-key")},
				},
				Allow: []AllowRule{{Source: "weather", Tools: []string{"get_weather"}}},
			},
		},
		RateLimits: []RateLimit{
			{Tenant: "acme", Source: "weather", Tool: "get_weather", RPM: 2},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func TestAuthenticate(t *testing.T) {
	e := testEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", nil)
	req.Header.Set("Authorization", "Bearer acme-key")
	tenant, err := e.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if tenant != "acme" {
		t.Fatalf("tenant = %q", tenant)
	}
}

func TestAuthenticateMissing(t *testing.T) {
	e := testEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", nil)
	if _, err := e.Authenticate(req); err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthenticateWrongKey(t *testing.T) {
	e := testEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", nil)
	req.Header.Set("X-API-Key", "wrong")
	if _, err := e.Authenticate(req); err == nil {
		t.Fatal("expected error")
	}
}

func TestTenantHeaderIgnored(t *testing.T) {
	e := testEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", nil)
	req.Header.Set("Authorization", "Bearer acme-key")
	req.Header.Set("x-tenant-id", "admin")
	tenant, err := e.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if tenant != "acme" {
		t.Fatalf("tenant = %q", tenant)
	}
}

func TestAuthorize(t *testing.T) {
	e := testEngine(t)
	if err := e.Authorize("acme", "weather__get_weather"); err != nil {
		t.Fatalf("acme weather: %v", err)
	}
	if err := e.Authorize("acme", "crm__get_customer"); err != nil {
		t.Fatalf("acme crm: %v", err)
	}
	if err := e.Authorize("acme", "crm__create_order"); err == nil {
		t.Fatal("expected denied")
	}
	if err := e.Authorize("beta", "weather__list_cities"); err == nil {
		t.Fatal("expected denied")
	}
}

func TestRateLimit(t *testing.T) {
	e := testEngine(t)
	for i := 0; i < 2; i++ {
		ok, layer := e.Allow("acme", "weather__get_weather")
		if !ok {
			t.Fatalf("unexpected deny at %d layer=%s", i, layer)
		}
	}
	ok, layer := e.Allow("acme", "weather__get_weather")
	if ok {
		t.Fatal("expected rate limit")
	}
	if layer != "tool-rpm" {
		t.Fatalf("layer = %q", layer)
	}
	if ok, _ := e.Allow("beta", "weather__get_weather"); !ok {
		t.Fatal("beta should not be limited by acme")
	}
}

func TestRateLimitDoesNotAffectOtherTools(t *testing.T) {
	e := testEngine(t)
	for i := 0; i < 2; i++ {
		if ok, _ := e.Allow("acme", "weather__get_weather"); !ok {
			t.Fatalf("unexpected deny at %d", i)
		}
	}
	if ok, _ := e.Allow("acme", "weather__list_cities"); !ok {
		t.Fatal("other tool should not be limited")
	}
}
