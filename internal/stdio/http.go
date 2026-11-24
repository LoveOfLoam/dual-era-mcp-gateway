package stdio

import (
	"encoding/json"
	"fmt"
	"net/http"

	"dual-era-mcp-gateway/internal/mcp"
)

// NewHTTPHandler exposes a bridge as a legacy MCP /mcp endpoint.
func NewHTTPHandler(b *Bridge, initResult json.RawMessage) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := mcp.ParseRequest(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeError(w, nil, mcp.CodeInvalidRequest, "bad json")
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "stdio")
			writeJSON(w, mcp.ResultResponse(req.ID, initResult))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "ping":
			writeJSON(w, mcp.ResultResponse(req.ID, map[string]any{}))
		case "tools/list":
			tools, err := b.ListTools(r.Context())
			if err != nil {
				writeError(w, req.ID, mcp.CodeInternal, err.Error())
				return
			}
			writeJSON(w, mcp.ResultResponse(req.ID, map[string]any{"tools": tools}))
		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
				writeError(w, req.ID, mcp.CodeInvalidParams, "name is required")
				return
			}
			resp, err := b.Call(r.Context(), params.Name, params.Arguments)
			if err != nil {
				writeError(w, req.ID, mcp.CodeInternal, err.Error())
				return
			}
			resp.JSONRPC = "2.0"
			resp.ID = req.ID
			writeJSON(w, resp)
		default:
			writeError(w, req.ID, mcp.CodeMethodNotFound, "method not supported by stdio bridge")
		}
	})
	return mux
}

func writeJSON(w http.ResponseWriter, resp mcp.Response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	writeJSON(w, mcp.ErrorResponse(id, code, fmt.Sprintf("stdio bridge: %s", msg)))
}
