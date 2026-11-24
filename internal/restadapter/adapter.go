// Package restadapter exposes a REST SourceSpec as a legacy MCP client so the
// aggregator/registry can treat REST tools exactly like native MCP tools.
package restadapter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
	"dual-era-mcp-gateway/internal/source"
)

var pathParamRE = regexp.MustCompile(`\{([^}]+)\}`)

// Adapter implements legacy.Client for one REST source.
type Adapter struct {
	spec        source.SourceSpec
	base        *url.URL
	tools       map[string]source.ToolSpec
	http        *http.Client
	tokenSource oauth2.TokenSource
}

// New creates an adapter. A nil client gets a 15s timeout.
func New(spec source.SourceSpec, client *http.Client) (*Adapter, error) {
	if spec.Type != source.TypeREST {
		return nil, fmt.Errorf("restadapter: source %q is not rest", spec.Name)
	}
	base, err := url.Parse(spec.BaseURL)
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("restadapter: invalid baseURL %q", spec.BaseURL)
	}
	if isMetadataHost(base.Hostname()) {
		return nil, fmt.Errorf("restadapter: metadata host %q is not allowed", base.Hostname())
	}
	if len(spec.AllowedHosts) > 0 && !hostAllowed(base.Hostname(), spec.AllowedHosts) {
		return nil, fmt.Errorf("restadapter: host %q is not in allowedHosts", base.Hostname())
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	var tokenSource oauth2.TokenSource
	if spec.Auth != nil {
		switch spec.Auth.Type {
		case "oauth2":
			if spec.Auth.TokenURL == "" || spec.Auth.ClientID == "" || spec.Auth.ClientSecret == "" {
				return nil, fmt.Errorf("restadapter: oauth2 requires tokenURL/clientID/clientSecret")
			}
			cfg := clientcredentials.Config{
				ClientID:     spec.Auth.ClientID,
				ClientSecret: spec.Auth.ClientSecret,
				TokenURL:     spec.Auth.TokenURL,
				Scopes:       spec.Auth.Scopes,
			}
			tokenSource = cfg.TokenSource(context.Background())
		case "hmac":
			if spec.Auth.Secret == "" || spec.Auth.Header == "" {
				return nil, fmt.Errorf("restadapter: hmac requires secret/header")
			}
		default:
			return nil, fmt.Errorf("restadapter: unsupported auth type %q", spec.Auth.Type)
		}
	}
	tools := make(map[string]source.ToolSpec, len(spec.Tools))
	for _, t := range spec.Tools {
		tools[t.Name] = t
	}
	return &Adapter{spec: spec, base: base, tools: tools, http: client, tokenSource: tokenSource}, nil
}

// ListTools returns unprefixed tools; the aggregator applies `source__tool`.
func (a *Adapter) ListTools(context.Context) ([]registry.Tool, error) {
	out := make([]registry.Tool, 0, len(a.spec.Tools))
	for _, t := range a.spec.Tools {
		out = append(out, registry.Tool{
			Name:         t.Name,
			OriginalName: t.Name,
			Description:  t.Description,
			InputSchema:  t.SchemaJSON(),
		})
	}
	return out, nil
}

// Call maps tool arguments to path/query/body and returns an MCP result.
func (a *Adapter) Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error) {
	tool, ok := a.tools[name]
	if !ok {
		return mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.Error{Code: mcp.CodeMethodNotFound, Message: "unknown tool"},
		}, nil
	}
	values := map[string]any{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &values); err != nil {
			return mcp.Response{
				JSONRPC: "2.0",
				Error:   &mcp.Error{Code: mcp.CodeInvalidParams, Message: "arguments must be an object"},
			}, nil
		}
	}

	path := tool.Path
	if path == "" {
		path = "/"
	}
	for _, match := range pathParamRE.FindAllStringSubmatch(path, -1) {
		key := match[1]
		v, ok := values[key]
		if !ok {
			return mcp.Response{
				JSONRPC: "2.0",
				Error:   &mcp.Error{Code: mcp.CodeInvalidParams, Message: "missing path parameter " + key},
			}, nil
		}
		path = strings.ReplaceAll(path, match[0], url.PathEscape(fmt.Sprint(v)))
		delete(values, key)
	}

	u := *a.base
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	method := strings.ToUpper(tool.Method)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	var bodyBytes []byte
	if method == http.MethodGet || method == http.MethodDelete {
		q := u.Query()
		for k, v := range values {
			q.Set(k, fmt.Sprint(v))
		}
		u.RawQuery = q.Encode()
	} else if len(values) > 0 {
		bodyBytes, _ = json.Marshal(values)
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return mcp.Response{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range a.spec.Headers {
		req.Header.Set(k, v)
	}
	if a.tokenSource != nil {
		tok, err := a.tokenSource.Token()
		if err != nil {
			return mcp.Response{}, fmt.Errorf("restadapter: oauth2 token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	if a.spec.Auth != nil && a.spec.Auth.Type == "hmac" {
		mac := hmac.New(sha256.New, []byte(a.spec.Auth.Secret))
		mac.Write(bodyBytes)
		req.Header.Set(a.spec.Auth.Header, hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return mcp.Response{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.Error{Code: mcp.CodeInternal, Message: fmt.Sprintf("REST upstream status %d", resp.StatusCode)},
		}, nil
	}
	result, _ := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(raw)}},
	})
	return mcp.Response{JSONRPC: "2.0", Result: result}, nil
}

// Close is a no-op.
func (a *Adapter) Close() error { return nil }

func isMetadataHost(host string) bool {
	return host == "169.254.169.254" || host == "metadata.google.internal"
}

func hostAllowed(host string, allowed []string) bool {
	for _, a := range allowed {
		if a == host {
			return true
		}
	}
	return false
}
