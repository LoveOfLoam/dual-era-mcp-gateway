// Package obs contains the minimal observability primitives used by the local
// gateway: Prometheus-style counters, JSON logs, and trace-id propagation.
package obs

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Metrics is a small lock-free counter set.
type Metrics struct {
	Requests    atomic.Int64
	Errors      atomic.Int64
	RateLimited atomic.Int64
	AuthDenied  atomic.Int64
	InFlight    atomic.Int64
}

// NewMetrics creates an empty metrics set.
func NewMetrics() *Metrics { return &Metrics{} }

// IncAuthDenied increments the auth-denied counter.
func (m *Metrics) IncAuthDenied() { m.AuthDenied.Add(1) }

// IncRateLimited increments the rate-limited counter.
func (m *Metrics) IncRateLimited() { m.RateLimited.Add(1) }

// Wrap counts requests, 5xx errors, and in-flight requests.
func (m *Metrics) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.Requests.Add(1)
		m.InFlight.Add(1)
		defer m.InFlight.Add(-1)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 500 {
			m.Errors.Add(1)
		}
	})
}

// Handler exposes the counters in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP mcp_gateway_requests_total Total HTTP requests.\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_requests_total counter\n")
		fmt.Fprintf(w, "mcp_gateway_requests_total %d\n", m.Requests.Load())
		fmt.Fprintf(w, "# HELP mcp_gateway_errors_total 5xx responses.\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_errors_total counter\n")
		fmt.Fprintf(w, "mcp_gateway_errors_total %d\n", m.Errors.Load())
		fmt.Fprintf(w, "# HELP mcp_gateway_rate_limited_total Rate-limited responses.\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_rate_limited_total counter\n")
		fmt.Fprintf(w, "mcp_gateway_rate_limited_total %d\n", m.RateLimited.Load())
		fmt.Fprintf(w, "# HELP mcp_gateway_auth_denied_total Auth-denied responses.\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_auth_denied_total counter\n")
		fmt.Fprintf(w, "mcp_gateway_auth_denied_total %d\n", m.AuthDenied.Load())
		fmt.Fprintf(w, "# HELP mcp_gateway_inflight_requests In-flight requests.\n")
		fmt.Fprintf(w, "# TYPE mcp_gateway_inflight_requests gauge\n")
		fmt.Fprintf(w, "mcp_gateway_inflight_requests %d\n", m.InFlight.Load())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
