// Package legacy implements a small MCP 2025-xx HTTP client used by the
// modern facade to bridge to legacy upstreams.
package legacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

// Client is the legacy upstream contract used by the facade.
type Client interface {
	ListTools(ctx context.Context) ([]registry.Tool, error)
	Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error)
	Close() error
}

// HTTPClient is a minimal legacy Streamable-HTTP MCP client. It initializes
// once, reuses Mcp-Session-Id, and retries once after a 404.
type HTTPClient struct {
	baseURL string
	http    *http.Client

	mu          sync.Mutex
	session     string
	initialized bool
	nextID      int
}

// NewHTTPClient creates a client for baseURL (for example http://host:8080/mcp).
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Close is a no-op for HTTP.
func (c *HTTPClient) Close() error { return nil }

// ListTools initializes if needed and returns tools from the legacy upstream.
// Names are returned unprefixed; the registry/aggregator applies the source
// prefix.
func (c *HTTPClient) ListTools(ctx context.Context) ([]registry.Tool, error) {
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	resp, err := c.callOnce(ctx, "tools/list", nil)
	if isNotFound(err) {
		c.reset()
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}
		resp, err = c.callOnce(ctx, "tools/list", nil)
	}
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("legacy tools/list: %s", resp.Error.Message)
	}
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	out := make([]registry.Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		out = append(out, registry.Tool{
			Name:         t.Name,
			OriginalName: t.Name,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
		})
	}
	return out, nil
}

// Call invokes a legacy tool. JSON-RPC errors are returned in the response, not
// as a Go error, so the facade can map them to the modern client.
func (c *HTTPClient) Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error) {
	if err := c.ensure(ctx); err != nil {
		return mcp.Response{}, err
	}
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = json.RawMessage(args)
	}
	resp, err := c.callOnce(ctx, "tools/call", params)
	if isNotFound(err) {
		c.reset()
		if err := c.ensure(ctx); err != nil {
			return mcp.Response{}, err
		}
		resp, err = c.callOnce(ctx, "tools/call", params)
	}
	return resp, err
}

func (c *HTTPClient) reset() {
	c.mu.Lock()
	c.session = ""
	c.initialized = false
	c.mu.Unlock()
}

func (c *HTTPClient) ensure(ctx context.Context) error {
	c.mu.Lock()
	if c.initialized {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	resp, session, err := c.post(ctx, "initialize", map[string]any{
		"protocolVersion": mcp.Version20250326,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "dual-era-mcp-gateway", "version": "0.1.0"},
	}, "")
	if err != nil {
		return fmt.Errorf("legacy initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("legacy initialize: %s", resp.Error.Message)
	}

	c.mu.Lock()
	c.session = session
	c.initialized = true
	c.mu.Unlock()

	// notifications/initialized has no id and no response body we care about.
	_, _, _ = c.post(ctx, "notifications/initialized", map[string]any{}, c.session)
	return nil
}

func (c *HTTPClient) callOnce(ctx context.Context, method string, params any) (mcp.Response, error) {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	resp, _, err := c.post(ctx, method, params, session)
	return resp, err
}

func (c *HTTPClient) post(ctx context.Context, method string, params any, session string) (mcp.Response, string, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	body := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		body["params"] = params
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return mcp.Response{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(raw))
	if err != nil {
		return mcp.Response{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return mcp.Response{}, "", err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return mcp.Response{}, resp.Header.Get("Mcp-Session-Id"), &HTTPStatusError{StatusCode: resp.StatusCode, Body: string(payload)}
	}
	if len(payload) == 0 {
		return mcp.Response{}, resp.Header.Get("Mcp-Session-Id"), nil
	}
	var rpc mcp.Response
	if err := json.Unmarshal(payload, &rpc); err != nil {
		return mcp.Response{}, resp.Header.Get("Mcp-Session-Id"), fmt.Errorf("decode response: %w", err)
	}
	return rpc, resp.Header.Get("Mcp-Session-Id"), nil
}

// HTTPStatusError carries a non-2xx status from a legacy upstream.
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("legacy upstream status %d: %s", e.StatusCode, e.Body)
}

func isNotFound(err error) bool {
	he, ok := err.(*HTTPStatusError)
	return ok && he.StatusCode == http.StatusNotFound
}
