package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type traceKey struct{}

// TraceID returns the W3C trace-id from traceparent or X-Trace-Id.
func TraceID(r *http.Request) string {
	if h := r.Header.Get("traceparent"); h != "" {
		parts := strings.Split(h, "-")
		if len(parts) == 4 && len(parts[1]) == 32 {
			if _, err := hex.DecodeString(parts[1]); err == nil {
				return parts[1]
			}
		}
	}
	if h := r.Header.Get("X-Trace-Id"); len(h) == 32 {
		if _, err := hex.DecodeString(h); err == nil {
			return h
		}
	}
	return ""
}

// TraceMiddleware propagates a trace id to the response and request context.
// It generates one if the caller did not supply a valid traceparent.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := TraceID(r)
		if id == "" {
			var b [16]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Trace-Id", id)
		next.ServeHTTP(w, r.WithContext(WithTraceID(r.Context(), id)))
	})
}

// TraceIDFromContext returns the trace id stored by TraceMiddleware.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceKey{}).(string); ok {
		return v
	}
	return ""
}

// WithTraceID stores a trace id for outbound clients.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey{}, id)
}
