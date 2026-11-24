// Package policy implements API-key authentication, tenant authorization, and
// a small in-memory rate limiter for the MVP.
package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
)

// APIKey stores only a SHA-256 digest.
type APIKey struct {
	Name   string `json:"name" yaml:"name"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

// AllowRule grants tools from one source. Tools may contain "*".
type AllowRule struct {
	Source string   `json:"source" yaml:"source"`
	Tools  []string `json:"tools" yaml:"tools"`
}

// Consumer is one authenticated caller.
type Consumer struct {
	Name    string      `json:"name" yaml:"name"`
	Tenant  string      `json:"tenant" yaml:"tenant"`
	APIKeys []APIKey    `json:"apiKeys" yaml:"apiKeys"`
	Allow   []AllowRule `json:"allow" yaml:"allow"`
}

// RateLimit is a per-tenant/source/tool RPM limit. Empty Source/Tool widen the
// scope.
type RateLimit struct {
	Tenant string `json:"tenant" yaml:"tenant"`
	Source string `json:"source" yaml:"source"`
	Tool   string `json:"tool" yaml:"tool"`
	RPM    int    `json:"rpm" yaml:"rpm"`
	Burst  int    `json:"burst" yaml:"burst"`
}

// Config is the policy input.
type Config struct {
	Consumers  []Consumer  `json:"consumers" yaml:"consumers"`
	RateLimits []RateLimit `json:"rateLimits" yaml:"rateLimits"`
	JWT        *JWTConfig  `json:"jwt,omitempty" yaml:"jwt,omitempty"`
}

type window struct {
	start time.Time
	count int
}

// Engine is a concurrency-safe policy engine.
type Engine struct {
	mu        sync.Mutex
	consumers map[string]Consumer
	keys      map[string]Consumer
	limits    []RateLimit
	windows   map[string]*window
	jwt       *JWTConfig
}

// New builds an Engine and validates key digests.
func New(cfg Config) (*Engine, error) {
	e := &Engine{
		consumers: make(map[string]Consumer),
		keys:      make(map[string]Consumer),
		limits:    cfg.RateLimits,
		windows:   make(map[string]*window),
		jwt:       cfg.JWT,
	}
	for _, c := range cfg.Consumers {
		if c.Tenant == "" {
			return nil, fmt.Errorf("consumer %q: tenant is required", c.Name)
		}
		if _, exists := e.consumers[c.Tenant]; exists {
			return nil, fmt.Errorf("duplicate tenant %q", c.Tenant)
		}
		for _, k := range c.APIKeys {
			if len(k.SHA256) != 64 {
				return nil, fmt.Errorf("consumer %q key %q: sha256 must be 64 hex chars", c.Name, k.Name)
			}
			if _, err := hex.DecodeString(k.SHA256); err != nil {
				return nil, fmt.Errorf("consumer %q key %q: invalid sha256", c.Name, k.Name)
			}
			e.keys[k.SHA256] = c
		}
		e.consumers[c.Tenant] = c
	}
	return e, nil
}

// Authenticate derives the tenant from the credential. It deliberately ignores
// x-tenant-id.
func (e *Engine) Authenticate(r *http.Request) (string, error) {
	key := bearerToken(r.Header.Get("Authorization"))
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	return e.AuthenticateToken(key)
}

// AuthenticateKey derives the tenant from a raw API key. ext_authz uses this
// without constructing a synthetic HTTP request.
func (e *Engine) AuthenticateKey(key string) (string, error) {
	if key == "" {
		return "", ErrUnauthenticated
	}
	sum := sha256.Sum256([]byte(key))
	digest := hex.EncodeToString(sum[:])
	e.mu.Lock()
	c, ok := e.keys[digest]
	e.mu.Unlock()
	if !ok {
		return "", ErrUnauthenticated
	}
	return c.Tenant, nil
}

// Authorize checks the tenant's allowlist for a normalized tool name.
func (e *Engine) Authorize(tenant, tool string) error {
	source, original, ok := splitTool(tool)
	if !ok {
		return ErrForbidden
	}
	e.mu.Lock()
	c, exists := e.consumers[tenant]
	e.mu.Unlock()
	if !exists {
		return ErrForbidden
	}
	for _, rule := range c.Allow {
		if rule.Source != "*" && rule.Source != source {
			continue
		}
		for _, allowed := range rule.Tools {
			if allowed == "*" || allowed == original {
				return nil
			}
		}
	}
	return ErrForbidden
}

// LimitFor returns the matching rate limit and layer for a tenant/tool.
func (e *Engine) LimitFor(tenant, tool string) (RateLimit, string, bool) {
	source, original, ok := splitTool(tool)
	if !ok {
		return RateLimit{}, "", false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.matchLimit(tenant, source, original)
}

// Allow checks and consumes one rate-limit slot. It returns the layer name for
// the 429 response.
func (e *Engine) Allow(tenant, tool string) (bool, string) {
	source, original, _ := splitTool(tool)
	e.mu.Lock()
	defer e.mu.Unlock()

	limit, layer, ok := e.matchLimit(tenant, source, original)
	if !ok || limit.RPM <= 0 {
		return true, ""
	}
	key := tenant + "|" + source + "|" + original + "|" + layer
	now := time.Now()
	w, exists := e.windows[key]
	if !exists || now.Sub(w.start) >= time.Minute {
		w = &window{start: now}
		e.windows[key] = w
	}
	if w.count >= limit.RPM {
		return false, layer
	}
	w.count++
	return true, ""
}

func (e *Engine) matchLimit(tenant, source, tool string) (RateLimit, string, bool) {
	best := -1
	var selected RateLimit
	for _, l := range e.limits {
		if l.Tenant != tenant {
			continue
		}
		if l.Source != "" && l.Source != source {
			continue
		}
		if l.Tool != "" && l.Tool != tool {
			continue
		}
		score := 0
		if l.Source != "" {
			score++
		}
		if l.Tool != "" {
			score++
		}
		if score > best {
			best = score
			selected = l
		}
	}
	if best < 0 {
		return RateLimit{}, "", false
	}
	layer := "tenant-rpm"
	if selected.Tool != "" {
		layer = "tool-rpm"
	} else if selected.Source != "" {
		layer = "source-rpm"
	}
	return selected, layer, true
}

func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func splitTool(tool string) (source, original string, ok bool) {
	i := strings.Index(tool, "__")
	if i <= 0 || i+2 >= len(tool) {
		return "", "", false
	}
	return tool[:i], tool[i+2:], true
}
