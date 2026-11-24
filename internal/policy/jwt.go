package policy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JWTConfig is the MVP HS256 validator configuration. RS256/JWKS is Phase 2.
type JWTConfig struct {
	HMACSecret  string `json:"hmacSecret" yaml:"hmacSecret"`
	Issuer      string `json:"issuer" yaml:"issuer"`
	Audience    string `json:"audience" yaml:"audience"`
	TenantClaim string `json:"tenantClaim" yaml:"tenantClaim"`
}

// AuthenticateToken tries JWT when configured and the token has three parts;
// otherwise it falls back to API-key authentication.
func (e *Engine) AuthenticateToken(token string) (string, error) {
	if e.jwt != nil && strings.Count(token, ".") == 2 {
		return e.AuthenticateJWT(token)
	}
	return e.AuthenticateKey(token)
}

// AuthenticateJWT validates an HS256 JWT and returns its tenant claim.
func (e *Engine) AuthenticateJWT(token string) (string, error) {
	if e.jwt == nil || e.jwt.HMACSecret == "" {
		return "", ErrUnauthenticated
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrUnauthenticated
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrUnauthenticated
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil || header.Alg != "HS256" {
		return "", ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, []byte(e.jwt.HMACSecret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expected, got) {
		return "", ErrUnauthenticated
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrUnauthenticated
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadRaw, &claims); err != nil {
		return "", ErrUnauthenticated
	}
	now := time.Now().Unix()
	if exp, ok := numericClaim(claims["exp"]); ok && now > exp+60 {
		return "", fmt.Errorf("%w: expired", ErrUnauthenticated)
	}
	if nbf, ok := numericClaim(claims["nbf"]); ok && now+60 < nbf {
		return "", fmt.Errorf("%w: not yet valid", ErrUnauthenticated)
	}
	if e.jwt.Issuer != "" && claims["iss"] != e.jwt.Issuer {
		return "", fmt.Errorf("%w: issuer", ErrUnauthenticated)
	}
	if e.jwt.Audience != "" && !audienceMatches(claims["aud"], e.jwt.Audience) {
		return "", fmt.Errorf("%w: audience", ErrUnauthenticated)
	}
	claim := e.jwt.TenantClaim
	if claim == "" {
		claim = "tenant"
	}
	tenant, _ := claims[claim].(string)
	if tenant == "" {
		tenant, _ = claims["sub"].(string)
	}
	if tenant == "" {
		return "", ErrUnauthenticated
	}
	return tenant, nil
}

func numericClaim(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func audienceMatches(v any, want string) bool {
	switch a := v.(type) {
	case string:
		return a == want
	case []any:
		for _, item := range a {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// ErrJWTNotConfigured is returned by callers that require JWT explicitly.
var ErrJWTNotConfigured = errors.New("jwt not configured")
