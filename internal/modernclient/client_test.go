package modernclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
)

func TestListAndCallHeaders(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("MCP-Protocol-Version") != mcp.Version20260728 {
			t.Errorf("version header = %q", r.Header.Get("MCP-Protocol-Version"))
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name string `json:"name"`
				Meta struct {
					Version string `json:"io.modelcontextprotocol/protocolVersion"`
				} `json:"_meta"`
			} `json:"params"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)
		if r.Header.Get("Mcp-Method") != req.Method {
			t.Errorf("method header = %q body=%q", r.Header.Get("Mcp-Method"), req.Method)
		}
		if req.Params.Meta.Version != mcp.Version20260728 {
			t.Errorf("meta version = %q", req.Params.Meta.Version)
		}
		switch req.Method {
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"x","description":"d","inputSchema":{"type":"object"}}]}}`))
		case "tools/call":
			if r.Header.Get("Mcp-Name") != "x" {
				t.Errorf("name header = %q", r.Header.Get("Mcp-Name"))
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[]}}`))
		default:
			http.Error(w, "bad method", 400)
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "x" {
		t.Fatalf("tools = %+v", tools)
	}
	resp, err := c.Call(context.Background(), "x", json.RawMessage(`{"a":1}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil || resp.Result == nil {
		t.Fatalf("resp = %+v", resp)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.ListTools(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
