package legacy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type mockUpstream struct {
	mu                sync.Mutex
	initCount         int
	listCount         int
	requireSess       bool
	session           string
	failFirstList     bool
	notificationHadID bool
}

func (m *mockUpstream) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		m.mu.Lock()
		defer m.mu.Unlock()

		switch req.Method {
		case "initialize":
			m.initCount++
			m.session = "sess-1"
			w.Header().Set("Mcp-Session-Id", m.session)
			writeResult(w, req.ID, map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "mock", "version": "1"},
			})
		case "notifications/initialized":
			m.notificationHadID = len(req.ID) > 0
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if m.requireSess && r.Header.Get("Mcp-Session-Id") != m.session {
				http.Error(w, "missing session", http.StatusNotFound)
				return
			}
			m.listCount++
			if m.failFirstList && m.listCount == 1 {
				http.Error(w, "expired session", http.StatusNotFound)
				return
			}
			writeResult(w, req.ID, map[string]any{
				"tools": []map[string]any{
					{"name": "get_weather", "description": "weather", "inputSchema": map[string]any{"type": "object"}},
				},
			})
		case "tools/call":
			if m.requireSess && r.Header.Get("Mcp-Session-Id") != m.session {
				http.Error(w, "missing session", http.StatusNotFound)
				return
			}
			writeResult(w, req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "ok"}},
			})
		default:
			writeError(w, req.ID, -32601, "unknown method")
		}
	})
}

func writeResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func TestInitializeAndListTools(t *testing.T) {
	m := &mockUpstream{requireSess: true}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "get_weather" {
		t.Fatalf("tools = %+v", tools)
	}
	if m.initCount != 1 {
		t.Fatalf("initCount = %d", m.initCount)
	}
	if m.notificationHadID {
		t.Fatal("notifications/initialized must not carry an id")
	}
}

func TestCallUsesSession(t *testing.T) {
	m := &mockUpstream{requireSess: true}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	resp, err := c.Call(context.Background(), "get_weather", json.RawMessage(`{"q":"Paris"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("missing result")
	}
}

func TestReinitializeOn404(t *testing.T) {
	m := &mockUpstream{requireSess: true, failFirstList: true}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %+v", tools)
	}
	if m.initCount != 2 {
		t.Fatalf("initCount = %d, want 2", m.initCount)
	}
}

func TestCallHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	if _, err := c.Call(context.Background(), "x", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCallJSONRPCErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "sess-1")
			writeResult(w, req.ID, map[string]any{"protocolVersion": "2025-03-26"})
			return
		}
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeError(w, req.ID, -32601, "no such tool")
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	resp, err := c.Call(context.Background(), "x", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("resp = %+v", resp)
	}
}
