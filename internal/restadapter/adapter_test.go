package restadapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/source"
)

func testSpec(baseURL string) source.SourceSpec {
	return source.SourceSpec{
		Name:    "crm",
		Type:    source.TypeREST,
		BaseURL: baseURL,
		Prefix:  "crm",
		Headers: map[string]string{"X-API-Key": "secret"},
		Tools: []source.ToolSpec{
			{
				Name:        "get_customer",
				Description: "get customer",
				Method:      "GET",
				Path:        "/v1/customers/{customer_id}",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"customer_id": map[string]any{"type": "string"}},
				},
			},
			{
				Name:        "create_order",
				Description: "create order",
				Method:      "POST",
				Path:        "/v1/orders",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
}

func TestListTools(t *testing.T) {
	a, err := New(testSpec("https://crm.example.com"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tools, err := a.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "get_customer" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestGetWithPathAndQuery(t *testing.T) {
	var (
		mu                        sync.Mutex
		gotPath, gotQuery, gotKey string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("X-API-Key")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","name":"Ada"}`))
	}))
	defer srv.Close()

	a, err := New(testSpec(srv.URL), srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := a.Call(context.Background(), "get_customer", json.RawMessage(`{"customer_id":"42","fields":"name"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v", resp.Error)
	}
	mu.Lock()
	path, query, key := gotPath, gotQuery, gotKey
	mu.Unlock()
	if path != "/v1/customers/42" {
		t.Fatalf("path = %q", path)
	}
	if query != "fields=name" {
		t.Fatalf("query = %q", query)
	}
	if key != "secret" {
		t.Fatalf("key = %q", key)
	}
}

func TestPostJSONBody(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		gotBody = body
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	a, _ := New(testSpec(srv.URL), srv.Client())
	resp, err := a.Call(context.Background(), "create_order", json.RawMessage(`{"sku":"x","qty":2}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v", resp.Error)
	}
	mu.Lock()
	body := gotBody
	mu.Unlock()
	if body["sku"] != "x" || body["qty"].(float64) != 2 {
		t.Fatalf("body = %+v", body)
	}
}

func TestOAuth2ClientCredentials(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token-1","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	var (
		mu      sync.Mutex
		gotAuth string
	)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer apiSrv.Close()

	spec := testSpec(apiSrv.URL)
	spec.Auth = &source.AuthSpec{Type: "oauth2", TokenURL: tokenSrv.URL, ClientID: "id", ClientSecret: "secret"}
	a, err := New(spec, apiSrv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Call(context.Background(), "get_customer", json.RawMessage(`{"customer_id":"42"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	mu.Lock()
	auth := gotAuth
	mu.Unlock()
	if auth != "Bearer token-1" {
		t.Fatalf("auth = %q", auth)
	}
}

func TestHMACSignature(t *testing.T) {
	const secret = "hmac-secret"
	var (
		mu     sync.Mutex
		gotSig string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		mu.Lock()
		gotSig = r.Header.Get("X-Signature")
		sig := gotSig
		mu.Unlock()
		if sig != hex.EncodeToString(mac.Sum(nil)) {
			t.Errorf("signature mismatch")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	spec := testSpec(srv.URL)
	spec.Auth = &source.AuthSpec{Type: "hmac", Secret: secret, Header: "X-Signature"}
	a, err := New(spec, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Call(context.Background(), "create_order", json.RawMessage(`{"sku":"x","qty":2}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	mu.Lock()
	sig := gotSig
	mu.Unlock()
	if sig == "" {
		t.Fatal("missing signature")
	}
}

func TestRejectsMetadataHost(t *testing.T) {
	if _, err := New(testSpec("http://169.254.169.254"), nil); err == nil {
		t.Fatal("expected metadata host rejection")
	}
}

func TestAllowedHosts(t *testing.T) {
	spec := testSpec("https://crm.example.com")
	spec.AllowedHosts = []string{"crm.example.com"}
	if _, err := New(spec, nil); err != nil {
		t.Fatalf("allowed host: %v", err)
	}
	spec.BaseURL = "https://evil.example.com"
	if _, err := New(spec, nil); err == nil {
		t.Fatal("expected allowlist rejection")
	}
}

func TestUnknownTool(t *testing.T) {
	a, _ := New(testSpec("https://crm.example.com"), nil)
	resp, err := a.Call(context.Background(), "missing", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.CodeMethodNotFound {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a, _ := New(testSpec(srv.URL), srv.Client())
	resp, err := a.Call(context.Background(), "get_customer", json.RawMessage(`{"customer_id":"42"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.CodeInternal {
		t.Fatalf("resp = %+v", resp)
	}
}
