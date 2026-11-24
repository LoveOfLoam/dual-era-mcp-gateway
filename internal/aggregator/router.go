// Package aggregator normalizes multiple legacy MCP clients into one catalog.
package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dual-era-mcp-gateway/internal/legacy"
	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

// Backend is one named legacy MCP source.
type Backend struct {
	Name   string
	Client legacy.Client
}

// Router aggregates legacy clients. It is the facade's Upstream.
type Router struct {
	backends map[string]legacy.Client
}

// New validates and builds a Router.
func New(backends []Backend) (*Router, error) {
	r := &Router{backends: make(map[string]legacy.Client, len(backends))}
	for _, b := range backends {
		if b.Name == "" || strings.Contains(b.Name, "__") {
			return nil, fmt.Errorf("invalid backend name %q", b.Name)
		}
		if b.Client == nil {
			return nil, fmt.Errorf("backend %q: nil client", b.Name)
		}
		if _, exists := r.backends[b.Name]; exists {
			return nil, fmt.Errorf("duplicate backend %q", b.Name)
		}
		r.backends[b.Name] = b.Client
	}
	return r, nil
}

// ListTools merges tools from all healthy backends. A single failing backend
// does not hide the others.
func (r *Router) ListTools(ctx context.Context) ([]registry.Tool, error) {
	sources := make(map[string][]registry.Tool, len(r.backends))
	var firstErr error
	for name, client := range r.backends {
		tools, err := client.ListTools(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sources[name] = tools
	}
	if len(sources) == 0 && firstErr != nil {
		return nil, firstErr
	}
	c, err := registry.Merge(sources)
	if err != nil {
		return nil, err
	}
	return c.List(), nil
}

// Call routes a normalized tool to its backend and strips the prefix.
func (r *Router) Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error) {
	source, original, ok := split(name)
	if !ok {
		return mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.Error{Code: mcp.CodeMethodNotFound, Message: "unknown tool"},
		}, nil
	}
	client, ok := r.backends[source]
	if !ok {
		return mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.Error{Code: mcp.CodeMethodNotFound, Message: "unknown tool"},
		}, nil
	}
	return client.Call(ctx, original, args)
}

// Health is a best-effort list of backend health.
func (r *Router) Health(ctx context.Context) map[string]error {
	out := make(map[string]error, len(r.backends))
	for name, client := range r.backends {
		_, err := client.ListTools(ctx)
		out[name] = err
	}
	return out
}

func split(tool string) (source, original string, ok bool) {
	i := strings.Index(tool, "__")
	if i <= 0 || i+2 >= len(tool) {
		return "", "", false
	}
	return tool[:i], tool[i+2:], true
}
