// Package edge is a local HTTP dispatcher for the two B1 endpoints. In
// production Envoy performs this path dispatch; this handler keeps local tests
// and the `cmd/gateway` demo simple.
package edge

import (
	"encoding/json"
	"net/http"
)

// Server dispatches /mcp and /modern/mcp.
type Server struct {
	legacy http.Handler
	modern http.Handler
}

// New creates an edge dispatcher.
func New(legacy, modern http.Handler) *Server {
	return &Server{legacy: legacy, modern: modern}
}

// ServeHTTP dispatches by path.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/mcp":
		s.legacy.ServeHTTP(w, r)
	case "/modern/mcp":
		s.modern.ServeHTTP(w, r)
	case "/healthz", "/readyz":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}
