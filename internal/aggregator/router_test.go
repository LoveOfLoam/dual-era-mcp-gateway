package aggregator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

type fakeClient struct {
	name  string
	tools []registry.Tool
	err   error
	calls []string
}

func (f *fakeClient) ListTools(context.Context) ([]registry.Tool, error) {
	return f.tools, f.err
}

func (f *fakeClient) Call(_ context.Context, name string, _ json.RawMessage) (mcp.Response, error) {
	f.calls = append(f.calls, name)
	if f.err != nil {
		return mcp.Response{}, f.err
	}
	return mcp.Response{JSONRPC: "2.0", Result: json.RawMessage(`{"content":[]}`)}, nil
}

func (f *fakeClient) Close() error { return nil }

func TestListToolsPrefixesAndSorts(t *testing.T) {
	w := &fakeClient{name: "weather", tools: []registry.Tool{{Name: "get_weather"}}}
	o := &fakeClient{name: "orders", tools: []registry.Tool{{Name: "create_order"}}}
	r, err := New([]Backend{{Name: "weather", Client: w}, {Name: "orders", Client: o}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "orders__create_order" || tools[1].Name != "weather__get_weather" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestCallRoutesAndStripsPrefix(t *testing.T) {
	w := &fakeClient{name: "weather"}
	o := &fakeClient{name: "orders"}
	r, _ := New([]Backend{{Name: "weather", Client: w}, {Name: "orders", Client: o}})
	if _, err := r.Call(context.Background(), "orders__create_order", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(o.calls) != 1 || o.calls[0] != "create_order" {
		t.Fatalf("orders calls = %+v", o.calls)
	}
	if len(w.calls) != 0 {
		t.Fatalf("weather calls = %+v", w.calls)
	}
}

func TestCallUnknownTool(t *testing.T) {
	r, _ := New([]Backend{{Name: "weather", Client: &fakeClient{name: "weather"}}})
	resp, err := r.Call(context.Background(), "missing", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.CodeMethodNotFound {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestPartialFailureKeepsHealthySources(t *testing.T) {
	w := &fakeClient{name: "weather", tools: []registry.Tool{{Name: "get_weather"}}}
	bad := &fakeClient{name: "orders", err: errors.New("down")}
	r, _ := New([]Backend{{Name: "weather", Client: w}, {Name: "orders", Client: bad}})
	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "weather__get_weather" {
		t.Fatalf("tools = %+v", tools)
	}
}
