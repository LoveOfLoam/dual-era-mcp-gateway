package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"dual-era-mcp-gateway/internal/policy"
)

// Limiter is the rate-limit backend used by the gRPC service.
type Limiter interface {
	Allow(ctx context.Context, tenant, tool string) (bool, string)
}

// MemoryLimiter uses the in-process policy engine.
type MemoryLimiter struct {
	engine *policy.Engine
}

// NewMemoryLimiter creates an in-process limiter.
func NewMemoryLimiter(engine *policy.Engine) MemoryLimiter {
	return MemoryLimiter{engine: engine}
}

// Allow implements Limiter.
func (m MemoryLimiter) Allow(_ context.Context, tenant, tool string) (bool, string) {
	return m.engine.Allow(tenant, tool)
}

const fixedWindowLua = `
local c = redis.call('INCR', KEYS[1])
if c == 1 then
  redis.call('EXPIRE', KEYS[1], 60)
end
if c > tonumber(ARGV[1]) then
  return 0
end
return 1
`

// RedisLimiter uses a Redis Lua fixed window and falls back to memory when
// Redis is unavailable. The fallback may over-admit; it is fail-open by design.
type RedisLimiter struct {
	engine   *policy.Engine
	client   *redis.Client
	fallback MemoryLimiter
}

// NewRedisLimiter creates a Redis-backed limiter.
func NewRedisLimiter(engine *policy.Engine, addr, password string, db int) *RedisLimiter {
	return &RedisLimiter{
		engine:   engine,
		client:   redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db}),
		fallback: NewMemoryLimiter(engine),
	}
}

// Close closes the Redis client.
func (r *RedisLimiter) Close() error { return r.client.Close() }

// Allow implements Limiter.
func (r *RedisLimiter) Allow(ctx context.Context, tenant, tool string) (bool, string) {
	limit, layer, ok := r.engine.LimitFor(tenant, tool)
	if !ok || limit.RPM <= 0 {
		return true, ""
	}
	key := fmt.Sprintf("mcp:rl:%s:%s:%s:%d", layer, tenant, tool, time.Now().Unix()/60)
	allowed, err := r.client.Eval(ctx, fixedWindowLua, []string{key}, limit.RPM).Int64()
	if err != nil {
		ok, fallbackLayer := r.fallback.Allow(ctx, tenant, tool)
		if fallbackLayer == "" {
			fallbackLayer = layer
		}
		return ok, fallbackLayer
	}
	return allowed == 1, layer
}
