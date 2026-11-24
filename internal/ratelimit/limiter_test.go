package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"dual-era-mcp-gateway/internal/policy"
)

func redisPolicy(t *testing.T) *policy.Engine {
	t.Helper()
	sum := sha256.Sum256([]byte("acme-key"))
	e, err := policy.New(policy.Config{
		Consumers: []policy.Consumer{{
			Name:    "acme",
			Tenant:  "acme",
			APIKeys: []policy.APIKey{{Name: "prod", SHA256: hex.EncodeToString(sum[:])}},
			Allow:   []policy.AllowRule{{Source: "weather", Tools: []string{"*"}}},
		}},
		RateLimits: []policy.RateLimit{{Tenant: "acme", Source: "weather", Tool: "get_weather", RPM: 1}},
	})
	if err != nil {
		t.Fatalf("policy.New: %v", err)
	}
	return e
}

func TestRedisLimiter(t *testing.T) {
	mr := miniredis.RunT(t)
	l := NewRedisLimiter(redisPolicy(t), mr.Addr(), "", 0)
	defer l.Close()

	ok, layer := l.Allow(context.Background(), "acme", "weather__get_weather")
	if !ok || layer != "tool-rpm" {
		t.Fatalf("first = %v %q", ok, layer)
	}
	ok, layer = l.Allow(context.Background(), "acme", "weather__get_weather")
	if ok || layer != "tool-rpm" {
		t.Fatalf("second = %v %q", ok, layer)
	}
}

func TestRedisLimiterFallback(t *testing.T) {
	mr := miniredis.RunT(t)
	l := NewRedisLimiter(redisPolicy(t), mr.Addr(), "", 0)
	defer l.Close()
	mr.Close()
	ok, _ := l.Allow(context.Background(), "acme", "weather__get_weather")
	if !ok {
		t.Fatal("fallback should allow first request")
	}
}
