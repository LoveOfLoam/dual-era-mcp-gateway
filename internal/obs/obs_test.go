package obs

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsWrapAndHandler(t *testing.T) {
	m := NewMetrics()
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if got := m.Requests.Load(); got != 1 {
		t.Fatalf("requests = %d", got)
	}
	if got := m.Errors.Load(); got != 1 {
		t.Fatalf("errors = %d", got)
	}
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{"mcp_gateway_requests_total 1", "mcp_gateway_errors_total 1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestTraceMiddleware(t *testing.T) {
	const trace = "4bf92f3577b34da6a3ce929d0e0e4736"
	var seen string
	h := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = TraceIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("traceparent", "00-"+trace+"-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen != trace {
		t.Fatalf("context trace = %q", seen)
	}
	if rec.Header().Get("X-Trace-Id") != trace {
		t.Fatalf("response trace = %q", rec.Header().Get("X-Trace-Id"))
	}
}

func TestJSONLogger(t *testing.T) {
	var buf bytes.Buffer
	NewJSONLogger(&buf).Info("hello", "tenant", "acme")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) || !strings.Contains(out, `"tenant":"acme"`) {
		t.Fatalf("log = %s", out)
	}
}
