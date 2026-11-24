package facade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

type fakeUpstream struct {
	mu       sync.Mutex
	tools    []registry.Tool
	result   json.RawMessage
	callName string
	err      error
}

func (f *fakeUpstream) ListTools(context.Context) ([]registry.Tool, error) {
	return f.tools, f.err
}

func (f *fakeUpstream) Call(_ context.Context, name string, _ json.RawMessage) (mcp.Response, error) {
	f.mu.Lock()
	f.callName = name
	f.mu.Unlock()
	if f.err != nil {
		return mcp.Response{}, f.err
	}
	if name == "missing" {
		return mcp.Response{Error: &mcp.Error{Code: mcp.CodeMethodNotFound, Message: "unknown tool"}}, nil
	}
	return mcp.Response{JSONRPC: "2.0", Result: f.result}, nil
}

func (f *fakeUpstream) calledName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callName
}

func newTestFacade(up Upstream) *Facade {
	return New(Config{
		ServerInfo:        ServerInfo{Name: "dual-era-mcp-gateway", Version: "0.1.0"},
		SupportedVersions: []string{mcp.Version20260728},
		TTLMs:             60000,
		CacheScope:        "public",
		AllowedOrigins:    []string{"http://localhost"},
		MaxBodyBytes:      1 << 20,
	}, up)
}

func meta(version string) string {
	return `"_meta":{"io.modelcontextprotocol/protocolVersion":"` + version + `","io.modelcontextprotocol/clientCapabilities":{}}`
}

func post(t *testing.T, f *Facade, method, params string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"%s","params":%s}`, method, params)
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.Version20260728)
	req.Header.Set("Mcp-Method", method)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestDiscover(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	rec := post(t, f, "server/discover", `{`+meta(mcp.Version20260728)+`}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	result := out["result"].(map[string]any)
	versions := result["supportedVersions"].([]any)
	if len(versions) != 1 || versions[0] != mcp.Version20260728 {
		t.Fatalf("versions = %+v", versions)
	}
	if rec.Header().Get("Mcp-Session-Id") != "" {
		t.Fatal("modern response must not set Mcp-Session-Id")
	}
}

func TestMissingMeta(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	rec := post(t, f, "tools/list", `{}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeInvalidParams {
		t.Fatalf("code = %d", code)
	}
}

func TestMissingCapabilities(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	params := `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}`
	rec := post(t, f, "tools/list", params, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeInvalidParams {
		t.Fatalf("code = %d", code)
	}
}

func TestWrongVersion(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	rec := post(t, f, "tools/list", `{`+meta("2025-03-26")+`}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeUnsupportedVersion {
		t.Fatalf("code = %d", code)
	}
}

func TestMethodHeaderMismatch(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	rec := post(t, f, "tools/call", `{"name":"x","arguments":{},`+meta(mcp.Version20260728)+`}`, map[string]string{"Mcp-Name": "x", "Mcp-Method": "tools/list"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeHeaderMismatch {
		t.Fatalf("code = %d", code)
	}
}

func TestNameHeaderMismatch(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	rec := post(t, f, "tools/call", `{"name":"x","arguments":{},`+meta(mcp.Version20260728)+`}`, map[string]string{"Mcp-Name": "y"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeHeaderMismatch {
		t.Fatalf("code = %d", code)
	}
}

func TestGetRejected(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	req := httptest.NewRequest(http.MethodGet, "/modern/mcp", nil)
	rec := httptest.NewRecorder()
	f.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestToolsListSorted(t *testing.T) {
	up := &fakeUpstream{tools: []registry.Tool{
		{Name: "weather__get_weather", OriginalName: "get_weather", Source: "weather"},
		{Name: "orders__create_order", OriginalName: "create_order", Source: "orders"},
	}}
	f := newTestFacade(up)
	rec := post(t, f, "tools/list", `{`+meta(mcp.Version20260728)+`}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	result := out["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools = %+v", tools)
	}
	first := tools[0].(map[string]any)["name"]
	second := tools[1].(map[string]any)["name"]
	if first != "orders__create_order" || second != "weather__get_weather" {
		t.Fatalf("order = %v, %v", first, second)
	}
	if result["ttlMs"] == nil || result["cacheScope"] == nil {
		t.Fatalf("missing cache fields: %+v", result)
	}
}

func TestToolsCallAddsResultType(t *testing.T) {
	up := &fakeUpstream{result: json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`)}
	f := newTestFacade(up)
	rec := post(t, f, "tools/call", `{"name":"weather__get_weather","arguments":{"q":"Paris"},`+meta(mcp.Version20260728)+`}`, map[string]string{"Mcp-Name": "weather__get_weather"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	result := out["result"].(map[string]any)
	if result["resultType"] != "complete" {
		t.Fatalf("resultType = %v", result["resultType"])
	}
	if up.calledName() != "weather__get_weather" {
		t.Fatalf("called = %q", up.calledName())
	}
}

func TestUnknownTool(t *testing.T) {
	f := newTestFacade(&fakeUpstream{result: json.RawMessage(`{}`)})
	rec := post(t, f, "tools/call", `{"name":"missing","arguments":{},`+meta(mcp.Version20260728)+`}`, map[string]string{"Mcp-Name": "missing"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeMethodNotFound {
		t.Fatalf("code = %d", code)
	}
}

func TestParseError(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	out := decode(t, rec)
	if code := int(out["error"].(map[string]any)["code"].(float64)); code != mcp.CodeParse {
		t.Fatalf("code = %d", code)
	}
}

func TestOriginDenied(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	f.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}
