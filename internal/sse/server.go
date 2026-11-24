// Package sse implements the legacy MCP SSE transport as a small shim in front
// of an internal Streamable-HTTP MCP endpoint. Envoy mcp_router does not serve
// GET /mcp, so the public listener routes GET/SSE here and keeps POST /mcp on
// the router.
package sse

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"dual-era-mcp-gateway/internal/mcp"
)

type session struct {
	id              string
	upstreamSession string
	events          chan []byte
	done            chan struct{}
	closeOne        sync.Once
}

func (s *session) close() { s.closeOne.Do(func() { close(s.done) }) }

// Server is an SSE transport shim.
type Server struct {
	upstream string
	client   *http.Client

	mu       sync.Mutex
	sessions map[string]*session
}

// New creates a shim for an internal MCP HTTP endpoint.
func New(upstream string) *Server {
	return &Server{
		upstream: upstream,
		client:   &http.Client{Timeout: 15 * time.Second},
		sessions: map[string]*session{},
	}
}

// Handler returns the shim HTTP mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleSSE)
	mux.HandleFunc("/mcp/message", s.handleMessage)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		http.Error(w, "SSE GET required", http.StatusMethodNotAllowed)
		return
	}
	id := newID()
	sess := &session{id: id, events: make(chan []byte, 16), done: make(chan struct{})}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.sessions, id)
		s.mu.Unlock()
		sess.close()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	endpoint := "/mcp/message?sessionId=" + url.QueryEscape(id)
	if _, err := fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sess.done:
			return
		case msg := <-sess.events:
			if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("sessionId")
	s.mu.Lock()
	sess := s.sessions[id]
	s.mu.Unlock()
	if sess == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	req, err := mcp.ParseRequest(bytes.NewReader(rawBody))
	if err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	resp, err := s.forward(r.Context(), rawBody, sess)
	if err != nil {
		resp = mcp.ErrorResponse(req.ID, mcp.CodeInternal, err.Error())
	}
	if req.IsNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	raw, _ := json.Marshal(resp)
	select {
	case sess.events <- raw:
	case <-time.After(2 * time.Second):
		http.Error(w, "session not reading", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) forward(ctx context.Context, raw []byte, sess *session) (mcp.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.upstream, bytes.NewReader(raw))
	if err != nil {
		return mcp.Response{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json, text/event-stream")
	if sess.upstreamSession != "" {
		hreq.Header.Set("Mcp-Session-Id", sess.upstreamSession)
	}
	resp, err := s.client.Do(hreq)
	if err != nil {
		return mcp.Response{}, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		sess.upstreamSession = sid
	}
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return mcp.Response{}, fmt.Errorf("upstream status %d: %s", resp.StatusCode, string(payload))
	}
	var out mcp.Response
	if err := json.Unmarshal(payload, &out); err != nil {
		return mcp.Response{}, err
	}
	return out, nil
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
