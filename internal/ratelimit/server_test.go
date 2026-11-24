package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	ratelimitcommon "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	"dual-era-mcp-gateway/internal/policy"
)

func keyHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func testPolicy(t *testing.T) *policy.Engine {
	t.Helper()
	e, err := policy.New(policy.Config{
		Consumers: []policy.Consumer{{
			Name:    "acme",
			Tenant:  "acme",
			APIKeys: []policy.APIKey{{Name: "prod", SHA256: keyHash("acme-key")}},
			Allow:   []policy.AllowRule{{Source: "weather", Tools: []string{"*"}}},
		}},
		RateLimits: []policy.RateLimit{
			{Tenant: "acme", Source: "weather", Tool: "get_weather", RPM: 1},
		},
	})
	if err != nil {
		t.Fatalf("policy.New: %v", err)
	}
	return e
}

func request(tenant, tool string) *ratelimitv3.RateLimitRequest {
	return &ratelimitv3.RateLimitRequest{Descriptors: []*ratelimitcommon.RateLimitDescriptor{{
		Entries: []*ratelimitcommon.RateLimitDescriptor_Entry{
			{Key: "tenant", Value: tenant},
			{Key: "tool", Value: tool},
		},
	}}}
}

func TestRateLimit(t *testing.T) {
	s := New(testPolicy(t))
	first, err := s.ShouldRateLimit(context.Background(), request("acme", "weather__get_weather"))
	if err != nil {
		t.Fatalf("ShouldRateLimit: %v", err)
	}
	if first.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
		t.Fatalf("first = %+v", first)
	}
	second, err := s.ShouldRateLimit(context.Background(), request("acme", "weather__get_weather"))
	if err != nil {
		t.Fatalf("ShouldRateLimit: %v", err)
	}
	if second.GetOverallCode() != ratelimitv3.RateLimitResponse_OVER_LIMIT {
		t.Fatalf("second = %+v", second)
	}
	if !strings.Contains(string(second.GetRawBody()), `"layer":"tool-rpm"`) {
		t.Fatalf("body = %s", second.GetRawBody())
	}
	foundRetry := false
	for _, h := range second.GetResponseHeadersToAdd() {
		if h.GetKey() == "retry-after" {
			foundRetry = true
		}
	}
	if !foundRetry {
		t.Fatal("missing retry-after header")
	}
}

func TestOtherToolNotLimited(t *testing.T) {
	s := New(testPolicy(t))
	if _, err := s.ShouldRateLimit(context.Background(), request("acme", "weather__get_weather")); err != nil {
		t.Fatalf("first: %v", err)
	}
	resp, err := s.ShouldRateLimit(context.Background(), request("acme", "weather__list_cities"))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if resp.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestMissingDescriptorAllows(t *testing.T) {
	s := New(testPolicy(t))
	resp, _ := s.ShouldRateLimit(context.Background(), &ratelimitv3.RateLimitRequest{})
	if resp.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
		t.Fatalf("resp = %+v", resp)
	}
}
