package stdio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPHandler(t *testing.T) {
	b, err := StartCommand(helperCommand())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initResult, err := b.Initialize(ctx)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	srv := httptest.NewServer(NewHTTPHandler(b, initResult))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`},
		{"list", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`},
		{"call", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var out map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out["result"] == nil {
				t.Fatalf("missing result: %+v", out)
			}
		})
	}
}
