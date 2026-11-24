package sse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dual-era-mcp-gateway/internal/mcp"
)

func fakeUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "upstream-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{}}}`))
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != "upstream-session" {
				t.Errorf("missing upstream session")
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"echo"}]}}`))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"no"}}`))
		}
	}))
}

func readEvent(t *testing.T, br *bufio.Reader) (string, string) {
	t.Helper()
	var event, data string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event != "" || data != "" {
				return event, data
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestSSETransport(t *testing.T) {
	upstream := fakeUpstream(t)
	defer upstream.Close()
	shim := httptest.NewServer(New(upstream.URL + "/mcp").Handler())
	defer shim.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, shim.URL+"/mcp", nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)
	event, endpoint := readEvent(t, br)
	if event != "endpoint" || endpoint == "" {
		t.Fatalf("endpoint event = %q %q", event, endpoint)
	}

	post := func(body string) {
		pr, err := http.Post(shim.URL+endpoint, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer pr.Body.Close()
		if pr.StatusCode != http.StatusAccepted {
			t.Fatalf("post status = %d", pr.StatusCode)
		}
	}
	post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	event, data := readEvent(t, br)
	if event != "message" {
		t.Fatalf("event = %q data=%q", event, data)
	}
	if !strings.Contains(data, "protocolVersion") {
		t.Fatalf("initialize response = %s", data)
	}

	post(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	event, data = readEvent(t, br)
	if event != "message" || !strings.Contains(data, "echo") {
		t.Fatalf("tools/list event = %q data=%q", event, data)
	}
}

func TestSSERequiresEventStream(t *testing.T) {
	shim := httptest.NewServer(New("http://127.0.0.1:1/mcp").Handler())
	defer shim.Close()
	resp, err := http.Get(shim.URL + "/mcp")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
