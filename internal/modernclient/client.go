// Package modernclient is a minimal MCP 2026-07-28 client. It is the Phase 2
// counterpart of legacy.HTTPClient and implements the same ListTools/Call
// contract so the aggregator can treat modern upstreams as another source.
package modernclient

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

// Client is a stateless 2026-07-28 MCP client.
type Client struct {
	baseURL string
	http    *http.Client
	mu      sync.Mutex
	nextID  int
}

// New creates a modern client.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// Close is a no-op.
func (c *Client) Close() error { return nil }

// ListTools calls tools/list with the required 2026-07-28 context.
func (c *Client) ListTools(ctx context.Context) ([]registry.Tool, error) {
	resp, err := c.post(ctx, "tools/list", nil, "")
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("modern tools/list: %s", resp.Error.Message)
	}
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode modern tools/list: %w", err)
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

// Call calls tools/call and returns the raw JSON-RPC response.
func (c *Client) Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = json.RawMessage(args)
	}
	return c.post(ctx, "tools/call", params, name)
}

// Discover calls server/discover. It is useful for conformance tests and
// capability checks; the aggregator does not need it.
func (c *Client) Discover(ctx context.Context) (json.RawMessage, error) {
	resp, err := c.post(ctx, "server/discover", nil, "")
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("modern discover: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *Client) post(ctx context.Context, method string, params any, toolName string) (mcp.Response, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if params == nil {
		body["params"] = map[string]any{"_meta": modernMeta()}
	} else {
		p := params.(map[string]any)
		p["_meta"] = modernMeta()
		body["params"] = p
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return mcp.Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(raw))
	if err != nil {
		return mcp.Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.Version20260728)
	req.Header.Set("Mcp-Method", method)
	if toolName != "" {
		req.Header.Set("Mcp-Name", toolName)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return mcp.Response{}, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return mcp.Response{}, fmt.Errorf("modern upstream status %d: %s", resp.StatusCode, string(payload))
	}
	var rpc mcp.Response
	if err := json.Unmarshal(payload, &rpc); err != nil {
		return mcp.Response{}, fmt.Errorf("decode modern response: %w", err)
	}
	return rpc, nil
}

func modernMeta() map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcp.Version20260728,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "dual-era-mcp-gateway", "version": "0.1.0"},
	}
}
