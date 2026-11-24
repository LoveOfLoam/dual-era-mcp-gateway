package boundary_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"dual-era-mcp-gateway/internal/aggregator"
	"dual-era-mcp-gateway/internal/edge"
	"dual-era-mcp-gateway/internal/facade"
	"dual-era-mcp-gateway/internal/legacy"
	"dual-era-mcp-gateway/internal/legacyendpoint"
	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

// mockMCP is a minimal legacy MCP upstream. It deliberately returns original
// tool names; the aggregator applies the source prefix.
func mockMCP(t *testing.T, name string, tools []string) *httptest.Server {
	t.Helper()
	session := "sess-" + name
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", session)
			writeRPC(w, req.ID, map[string]any{
				"protocolVersion": mcp.Version20250618,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": name, "version": "1"},
			}, nil)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != session {
				http.Error(w, "missing session", http.StatusNotFound)
				return
			}
			list := make([]map[string]any, 0, len(tools))
			for _, tool := range tools {
				list = append(list, map[string]any{
					"name":        tool,
					"description": name + " " + tool,
					"inputSchema": map[string]any{"type": "object"},
				})
			}
			writeRPC(w, req.ID, map[string]any{"tools": list}, nil)
		case "tools/call":
			if r.Header.Get("Mcp-Session-Id") != session {
				http.Error(w, "missing session", http.StatusNotFound)
				return
			}
			var p struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(req.Params, &p)
			writeRPC(w, req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": name + ":" + p.Name}},
			}, nil)
		default:
			http.Error(w, "unknown method", 404)
		}
	}))
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, errObj any) {
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"jsonrpc": "2.0", "id": id}
	if errObj != nil {
		out["error"] = errObj
	} else {
		out["result"] = result
	}
	_ = json.NewEncoder(w).Encode(out)
}

func newDualEra(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	weather := mockMCP(t, "weather", []string{"get_weather"})
	crm := mockMCP(t, "crm", []string{"get_customer"})
	router, err := aggregator.New([]aggregator.Backend{
		{Name: "weather", Client: legacy.NewHTTPClient(weather.URL)},
		{Name: "crm", Client: legacy.NewHTTPClient(crm.URL)},
	})
	if err != nil {
		t.Fatalf("aggregator.New: %v", err)
	}
	legacySrv := legacyendpoint.New(legacyendpoint.Config{
		ServerInfo:      legacyendpoint.ServerInfo{Name: "legacy", Version: "1"},
		ProtocolVersion: mcp.Version20250618,
		SessionID:       "sess-public",
	}, router)
	modern := facade.New(facade.Config{
		ServerInfo:        facade.ServerInfo{Name: "modern", Version: "1"},
		SupportedVersions: []string{mcp.Version20260728},
		TTLMs:             60000,
		CacheScope:        "public",
	}, router)
	edgeSrv := httptest.NewServer(edge.New(legacySrv, modern))
	return edgeSrv, func() {
		edgeSrv.Close()
		weather.Close()
		crm.Close()
	}
}

func modernPost(t *testing.T, base, method, name, params string) (*http.Response, map[string]any) {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"%s","params":%s}`, method, params)
	req, _ := http.NewRequest(http.MethodPost, base+"/modern/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.Version20260728)
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func meta() string {
	return `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
}

func TestDualEraBothEndpoints(t *testing.T) {
	srv, cleanup := newDualEra(t)
	defer cleanup()

	// Modern discover.
	resp, out := modernPost(t, srv.URL, "server/discover", "", `{`+meta()+`}`)
	if resp.StatusCode != 200 {
		t.Fatalf("modern discover status=%d body=%+v", resp.StatusCode, out)
	}

	// Modern tools/list sees both native sources.
	_, out = modernPost(t, srv.URL, "tools/list", "", `{`+meta()+`}`)
	result := out["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools = %+v", tools)
	}
	names := map[string]bool{}
	for _, raw := range tools {
		names[raw.(map[string]any)["name"].(string)] = true
	}
	if !names["weather__get_weather"] || !names["crm__get_customer"] {
		t.Fatalf("names = %+v", names)
	}

	// Modern tools/call bridges to a legacy upstream.
	_, out = modernPost(t, srv.URL, "tools/call", "weather__get_weather", `{"name":"weather__get_weather","arguments":{"q":"Paris"},`+meta()+`}`)
	result = out["result"].(map[string]any)
	if result["resultType"] != "complete" {
		t.Fatalf("resultType = %v", result["resultType"])
	}

	// Legacy initialize + list + call.
	legacyBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	lr, err := http.Post(srv.URL+"/mcp", "application/json", bytes.NewBufferString(legacyBody))
	if err != nil {
		t.Fatalf("legacy initialize: %v", err)
	}
	session := lr.Header.Get("Mcp-Session-Id")
	lr.Body.Close()
	if session == "" {
		t.Fatal("missing legacy session")
	}

	legacyList := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", bytes.NewBufferString(legacyList))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", session)
	lr, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("legacy list: %v", err)
	}
	raw, _ := io.ReadAll(lr.Body)
	lr.Body.Close()
	var legacyOut map[string]any
	_ = json.Unmarshal(raw, &legacyOut)
	legacyTools := legacyOut["result"].(map[string]any)["tools"].([]any)
	if len(legacyTools) != 2 {
		t.Fatalf("legacy tools = %+v", legacyTools)
	}
}

func TestSingleSourcePrefixStable(t *testing.T) {
	// The aggregator prefixes even a single source; this is the behavior the
	// facade relies on and the opposite of Envoy mcp_router's single-backend
	// behavior noted in the spike.
	router, err := aggregator.New([]aggregator.Backend{{Name: "only", Client: &staticClient{tools: []registry.Tool{{Name: "x"}}}}})
	if err != nil {
		t.Fatalf("aggregator.New: %v", err)
	}
	tools, err := router.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "only__x" {
		t.Fatalf("tools = %+v", tools)
	}
}

type staticClient struct{ tools []registry.Tool }

func (s *staticClient) ListTools(context.Context) ([]registry.Tool, error) {
	return s.tools, nil
}
func (s *staticClient) Call(context.Context, string, json.RawMessage) (mcp.Response, error) {
	return mcp.Response{}, nil
}
func (s *staticClient) Close() error { return nil }
