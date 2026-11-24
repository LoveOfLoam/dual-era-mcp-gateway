// Package ratelimit implements Envoy's RateLimitService gRPC service with a
// memory limiter and an optional Redis Lua fixed-window limiter.
package ratelimit

import (
	"context"
	"encoding/json"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	"dual-era-mcp-gateway/internal/audit"
	"dual-era-mcp-gateway/internal/policy"
)

// Server implements ratelimitv3.RateLimitServiceServer.
type Server struct {
	ratelimitv3.UnimplementedRateLimitServiceServer
	limiter Limiter
	audit   *audit.Logger
}

// New creates a rate-limit server with the in-process limiter.
func New(engine *policy.Engine) *Server {
	return &Server{limiter: NewMemoryLimiter(engine)}
}

// NewWithLimiter creates a rate-limit server with a custom backend.
func NewWithLimiter(l Limiter) *Server {
	return &Server{limiter: l}
}

// WithAudit enables JSONL audit events.
func (s *Server) WithAudit(l *audit.Logger) *Server {
	s.audit = l
	return s
}

// ShouldRateLimit checks all descriptors; one over-limit descriptor rejects the
// request. Descriptors are expected to carry `tenant` and `tool`.
func (s *Server) ShouldRateLimit(ctx context.Context, req *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error) {
	for _, d := range req.GetDescriptors() {
		var tenant, tool string
		for _, e := range d.GetEntries() {
			switch e.GetKey() {
			case "tenant":
				tenant = e.GetValue()
			case "tool":
				tool = e.GetValue()
			}
		}
		if tenant == "" || tool == "" {
			continue
		}
		allowed, layer := s.limiter.Allow(ctx, tenant, tool)
		if !allowed {
			if s.audit != nil {
				s.audit.Emit(audit.Event{
					Tenant:   tenant,
					Consumer: tenant,
					Tool:     tool,
					Decision: "rate_limited",
					Layer:    layer,
					Status:   "over_limit",
				})
			}
			body, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"error": map[string]any{
					"code":    -32001,
					"message": "rate limited",
					"layer":   layer,
				},
			})
			return &ratelimitv3.RateLimitResponse{
				OverallCode: ratelimitv3.RateLimitResponse_OVER_LIMIT,
				RawBody:     body,
				ResponseHeadersToAdd: []*corev3.HeaderValue{
					{Key: "retry-after", Value: "60"},
					{Key: "x-ratelimit-layer", Value: layer},
				},
			}, nil
		}
	}
	return &ratelimitv3.RateLimitResponse{OverallCode: ratelimitv3.RateLimitResponse_OK}, nil
}
