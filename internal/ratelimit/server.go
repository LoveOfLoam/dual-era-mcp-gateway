// Package ratelimit implements Envoy's RateLimitService gRPC service using the
// in-process policy engine. Redis-backed limiting is a separate Phase 2 item.
package ratelimit

import (
	"context"
	"encoding/json"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	"dual-era-mcp-gateway/internal/policy"
)

// Server implements ratelimitv3.RateLimitServiceServer.
type Server struct {
	ratelimitv3.UnimplementedRateLimitServiceServer
	policy *policy.Engine
}

// New creates a rate-limit server.
func New(engine *policy.Engine) *Server {
	return &Server{policy: engine}
}

// ShouldRateLimit checks all descriptors; one over-limit descriptor rejects the
// request. Descriptors are expected to carry `tenant` and `tool`.
func (s *Server) ShouldRateLimit(_ context.Context, req *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error) {
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
		allowed, layer := s.policy.Allow(tenant, tool)
		if !allowed {
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
