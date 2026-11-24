package legacyendpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dual-era-mcp-gateway/internal/aggregator"
	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

type fakeClient struct {
	tools []registry.Tool
}

func (f *fakeClient) ListTools(context.Context) ([]registry.Tool, error) { return f.tools, nil }
func (f *fakeClient) Call(_ context.Context, name string, _ json.RawMessage) (mcp.Response, error) {
	return mcp.ResultResponse(json.RawMessage(`1`), map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok:" + name}}}), nil
}
func (f *fakeClient) Close() error { return nil }

func newServer(t *testing.T) *Server {
	t.Helper()
	router, err := aggregator.New([]aggregator.Backend{{
		Name: "weather",
		Client: &fakeClient{
			tools: []registry.Tool{{Name: "get_weather"}},
		},
	}})
	if err != nil {
		t.Fatalf("aggregator.New: %v", err)
	}
	return New(Config{ServerInfo: ServerInfo{Name: "legacy", Version: "1"}, ProtocolVersion: mcp.Version20250618, SessionID: "sess-1"}, router)
}

func post(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestInitialize(t *testing.T) {
	s := newServer(t)
	rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Mcp-Session-Id") != "sess-1" {
		t.Fatalf("session = %q", rec.Header().Get("Mcp-Session-Id"))
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	result := out["result"].(map[string]any)
	if result["protocolVersion"] != mcp.Version20250618 {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
}

func TestNotification(t *testing.T) {
	s := newServer(t)
	rec := post(t, s, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestToolsList(t *testing.T) {
	s := newServer(t)
	rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "weather__get_weather" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestToolsCall(t *testing.T) {
	s := newServer(t)
	rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"weather__get_weather","arguments":{"q":"Paris"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["result"] == nil {
		t.Fatalf("result = %+v", out)
	}
}

func TestGetRejected(t *testing.T) {
	s := newServer(t)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}
