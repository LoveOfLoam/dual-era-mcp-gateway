package policy

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// JWTConfig supports HS256 (HMACSecret), RS256 (PublicKeyPEM) and RS256 via a
// JWKS URL. JWKS keys are cached for five minutes.
type JWTConfig struct {
	HMACSecret   string `json:"hmacSecret" yaml:"hmacSecret"`
	PublicKeyPEM string `json:"publicKeyPEM" yaml:"publicKeyPEM"`
	JWKSURL      string `json:"jwksURL" yaml:"jwksURL"`
	Issuer       string `json:"issuer" yaml:"issuer"`
	Audience     string `json:"audience" yaml:"audience"`
	TenantClaim  string `json:"tenantClaim" yaml:"tenantClaim"`
}

// AuthenticateToken tries JWT when configured and the token has three parts;
// otherwise it falls back to API-key authentication.
func (e *Engine) AuthenticateToken(token string) (string, error) {
	if e.jwt != nil && strings.Count(token, ".") == 2 {
		return e.AuthenticateJWT(token)
	}
	return e.AuthenticateKey(token)
}

// AuthenticateJWT validates an HS256/RS256 JWT and returns its tenant claim.
func (e *Engine) AuthenticateJWT(token string) (string, error) {
	if e.jwt == nil {
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
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return "", ErrUnauthenticated
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrUnauthenticated
	}
	switch header.Alg {
	case "HS256":
		if e.jwt.HMACSecret == "" {
			return "", ErrUnauthenticated
		}
		mac := hmac.New(sha256.New, []byte(e.jwt.HMACSecret))
		mac.Write([]byte(parts[0] + "." + parts[1]))
		if !hmac.Equal(mac.Sum(nil), sig) {
			return "", ErrUnauthenticated
		}
	case "RS256":
		pub, err := e.rsaKey(header.Kid)
		if err != nil {
			return "", ErrUnauthenticated
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
			return "", ErrUnauthenticated
		}
	default:
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

func (e *Engine) rsaKey(kid string) (*rsa.PublicKey, error) {
	if e.publicKey != nil {
		return e.publicKey, nil
	}
	if e.jwks != nil {
		return e.jwks.key(kid)
	}
	return nil, errors.New("no RSA key configured")
}

func parseRSAPublicKey(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("invalid PEM")
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if rsaPub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}
	return nil, errors.New("not an RSA public key")
}

type jwksCache struct {
	url  string
	http *http.Client

	mu      sync.Mutex
	fetched time.Time
	keys    map[string]*rsa.PublicKey
}

func (c *jwksCache) key(kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetched) < 5*time.Minute {
		if k, ok := c.keys[kid]; ok {
			return k, nil
		}
	}
	if err := c.refreshLocked(); err != nil {
		return nil, err
	}
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	if kid == "" && len(c.keys) == 1 {
		for _, k := range c.keys {
			return k, nil
		}
	}
	return nil, errors.New("kid not found")
}

func (c *jwksCache) refreshLocked() error {
	if c.http == nil {
		c.http = &http.Client{Timeout: 5 * time.Second}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.N == "" || k.E == "" {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return errors.New("jwks has no RSA keys")
	}
	c.keys = keys
	c.fetched = time.Now()
	return nil
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
