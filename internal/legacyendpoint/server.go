// Package legacyendpoint serves the legacy /mcp endpoint for local tests and
// for environments that do not run Envoy. Production uses Envoy mcp_router.
package legacyendpoint

import (
	"encoding/json"
	"net/http"

	"dual-era-mcp-gateway/internal/aggregator"
	"dual-era-mcp-gateway/internal/mcp"
)

// ServerInfo identifies the legacy endpoint.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Config configures the legacy endpoint.
type Config struct {
	ServerInfo      ServerInfo
	ProtocolVersion string
	SessionID       string
}

// Server implements a minimal legacy MCP endpoint over an aggregator.
type Server struct {
	cfg    Config
	router *aggregator.Router
}

// New creates a legacy endpoint.
func New(cfg Config, router *aggregator.Router) *Server {
	if cfg.ProtocolVersion == "" {
		cfg.ProtocolVersion = mcp.Version20250618
	}
	if cfg.SessionID == "" {
		cfg.SessionID = "sess-local"
	}
	return &Server{cfg: cfg, router: router}
}

// ServeHTTP implements POST /mcp.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := mcp.ParseRequest(r.Body)
	if err != nil {
		writeError(w, nil, http.StatusBadRequest, mcp.CodeParse, "parse error")
		return
	}
	if req.IsNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch req.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", s.cfg.SessionID)
		s.writeResult(w, req, map[string]any{
			"protocolVersion": s.cfg.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      s.cfg.ServerInfo,
		})
	case "tools/list":
		tools, err := s.router.ListTools(r.Context())
		if err != nil {
			writeError(w, req.ID, http.StatusBadGateway, mcp.CodeInternal, "tools/list failed")
			return
		}
		s.writeResult(w, req, map[string]any{"tools": tools})
	case "tools/call":
		name := req.ToolName()
		if name == "" {
			writeError(w, req.ID, http.StatusBadRequest, mcp.CodeInvalidParams, "missing params.name")
			return
		}
		var p struct {
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		resp, err := s.router.Call(r.Context(), name, p.Arguments)
		if err != nil {
			writeError(w, req.ID, http.StatusBadGateway, mcp.CodeInternal, "tools/call failed")
			return
		}
		if resp.Error != nil {
			writeJSON(w, http.StatusOK, mcp.Response{JSONRPC: "2.0", ID: req.ID, Error: resp.Error})
			return
		}
		writeJSON(w, http.StatusOK, mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: resp.Result})
	default:
		writeError(w, req.ID, http.StatusOK, mcp.CodeMethodNotFound, "method not found")
	}
}

func (s *Server) writeResult(w http.ResponseWriter, req mcp.Request, result any) {
	writeJSON(w, http.StatusOK, mcp.ResultResponse(req.ID, result))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, id json.RawMessage, status, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(w, status, mcp.ErrorResponse(id, code, msg))
}
