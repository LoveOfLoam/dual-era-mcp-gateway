package admission

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func review(kind string, object string) string {
	return `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"u1","kind":{"group":"gateway.dual-era.io","version":"v1alpha1","kind":"` + kind + `"},"object":` + object + `}}`
}

func post(t *testing.T, body string) ReviewResponse {
	t.Helper()
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out ReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestValidSource(t *testing.T) {
	out := post(t, review("McpSource", `{"spec":{"name":"weather","type":"mcp","endpoint":"http://weather:9001/mcp","prefix":"weather"}}`))
	if !out.Response.Allowed {
		t.Fatalf("denied: %+v", out.Response.Status)
	}
}

func TestInvalidSource(t *testing.T) {
	out := post(t, review("McpSource", `{"spec":{"name":"weather","type":"unknown"}}`))
	if out.Response.Allowed {
		t.Fatal("expected denial")
	}
	if !strings.Contains(out.Response.Status.Message, "unknown type") {
		t.Fatalf("message = %q", out.Response.Status.Message)
	}
}

func TestConsumerBadKey(t *testing.T) {
	out := post(t, review("McpConsumer", `{"spec":{"tenant":"acme","apiKeys":[{"name":"prod","sha256":"short"}]}}`))
	if out.Response.Allowed {
		t.Fatal("expected denial")
	}
}

func TestBadJSON(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
