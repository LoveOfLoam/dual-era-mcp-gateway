// Package facade implements the 2026-07-28 modern MCP endpoint.
package facade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

// Upstream is the normalized internal MCP catalog the facade bridges to.
// It is usually the legacy router, but tests can provide a fake.
type Upstream interface {
	ListTools(ctx context.Context) ([]registry.Tool, error)
	Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error)
}

// Policy is the optional auth/authorization/rate-limit hook.
type Policy interface {
	Authenticate(r *http.Request) (tenant string, err error)
	Authorize(tenant, tool string) error
	Allow(tenant, tool string) (allowed bool, layer string)
}

// ServerInfo identifies the gateway in server/discover responses.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Config controls modern endpoint behavior.
type Config struct {
	ServerInfo        ServerInfo
	SupportedVersions []string
	TTLMs             int
	CacheScope        string
	AllowedOrigins    []string
	MaxBodyBytes      int64
	Policy            Policy
}

// Facade is the modern MCP HTTP handler.
type Facade struct {
	cfg      Config
	upstream Upstream
}

// New creates a facade. Zero values are replaced with safe defaults.
func New(cfg Config, up Upstream) *Facade {
	if len(cfg.SupportedVersions) == 0 {
		cfg.SupportedVersions = []string{mcp.Version20260728}
	}
	if cfg.TTLMs <= 0 {
		cfg.TTLMs = 60000
	}
	if cfg.CacheScope == "" {
		cfg.CacheScope = "public"
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "dual-era-mcp-gateway"
		cfg.ServerInfo.Version = "0.1.0"
	}
	return &Facade{cfg: cfg, upstream: up}
}

// ServeHTTP implements POST /modern/mcp.
func (f *Facade) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !f.originAllowed(r.Header.Get("Origin")) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
		return
	}

	tenant := ""
	if f.cfg.Policy != nil {
		t, err := f.cfg.Policy.Authenticate(r)
		if err != nil {
			writeError(w, nil, http.StatusUnauthorized, mcp.CodeUnauthorized, "unauthenticated")
			return
		}
		tenant = t
	}

	body := http.MaxBytesReader(w, r.Body, f.cfg.MaxBodyBytes)
	req, err := mcp.ParseRequest(body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, nil, http.StatusRequestEntityTooLarge, mcp.CodeInvalidRequest, "request too large")
			return
		}
		writeError(w, nil, http.StatusBadRequest, mcp.CodeParse, "parse error")
		return
	}
	if perr := mcp.ValidateModern(req, r.Header); perr != nil {
		writeError(w, req.ID, perr.HTTPStatus, perr.Code, perr.Message)
		return
	}
	if req.IsNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "server/discover":
		f.discover(w, req)
	case "tools/list":
		f.listTools(w, r, req, tenant)
	case "tools/call":
		f.callTool(w, r, req, tenant)
	default:
		writeError(w, req.ID, http.StatusOK, mcp.CodeMethodNotFound, "method not found")
	}
}

func (f *Facade) discover(w http.ResponseWriter, req mcp.Request) {
	result := map[string]any{
		"supportedVersions": f.cfg.SupportedVersions,
		"capabilities":      map[string]any{"tools": map[string]any{}},
		"serverInfo":        f.cfg.ServerInfo,
		"ttlMs":             f.cfg.TTLMs,
		"cacheScope":        f.cfg.CacheScope,
	}
	writeJSON(w, http.StatusOK, mcp.ResultResponse(req.ID, result))
}

func (f *Facade) listTools(w http.ResponseWriter, r *http.Request, req mcp.Request, tenant string) {
	tools, err := f.upstream.ListTools(r.Context())
	if err != nil {
		writeError(w, req.ID, http.StatusBadGateway, mcp.CodeInternal, "upstream tools/list failed")
		return
	}
	if f.cfg.Policy != nil {
		filtered := tools[:0]
		for _, t := range tools {
			if f.cfg.Policy.Authorize(tenant, t.Name) == nil {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	result := map[string]any{
		"tools":      tools,
		"ttlMs":      f.cfg.TTLMs,
		"cacheScope": f.cfg.CacheScope,
	}
	writeJSON(w, http.StatusOK, mcp.ResultResponse(req.ID, result))
}

func (f *Facade) callTool(w http.ResponseWriter, r *http.Request, req mcp.Request, tenant string) {
	name := req.ToolName()
	if name == "" {
		writeError(w, req.ID, http.StatusBadRequest, mcp.CodeInvalidParams, "missing params.name")
		return
	}
	if f.cfg.Policy != nil {
		if err := f.cfg.Policy.Authorize(tenant, name); err != nil {
			writeError(w, req.ID, http.StatusForbidden, mcp.CodeForbidden, "tool not allowed")
			return
		}
		if allowed, layer := f.cfg.Policy.Allow(tenant, name); !allowed {
			writeRateLimited(w, req.ID, layer)
			return
		}
	}
	var p struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			writeError(w, req.ID, http.StatusBadRequest, mcp.CodeInvalidParams, "invalid params")
			return
		}
	}
	resp, err := f.upstream.Call(r.Context(), name, p.Arguments)
	if err != nil {
		writeError(w, req.ID, http.StatusBadGateway, mcp.CodeInternal, "upstream tools/call failed")
		return
	}
	if resp.Error != nil {
		writeJSON(w, http.StatusOK, mcp.Response{JSONRPC: "2.0", ID: req.ID, Error: resp.Error})
		return
	}
	result, err := addResultType(resp.Result)
	if err != nil {
		writeError(w, req.ID, http.StatusBadGateway, mcp.CodeInternal, "invalid upstream result")
		return
	}
	writeJSON(w, http.StatusOK, mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func addResultType(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal(map[string]any{"resultType": "complete"})
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["resultType"] = "complete"
	return json.Marshal(m)
}

func (f *Facade) originAllowed(origin string) bool {
	if origin == "" || len(f.cfg.AllowedOrigins) == 0 {
		return true
	}
	for _, o := range f.cfg.AllowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, id json.RawMessage, status, code int, message string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(w, status, mcp.ErrorResponse(id, code, message))
}

func writeRateLimited(w http.ResponseWriter, id json.RawMessage, layer string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(w, http.StatusTooManyRequests, mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcp.Error{Code: mcp.CodeUnauthorized, Message: "rate limited", Layer: layer},
	})
}
