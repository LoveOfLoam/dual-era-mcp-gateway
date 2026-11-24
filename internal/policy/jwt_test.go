package policy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func signHS256(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func jwtEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New(Config{JWT: &JWTConfig{
		HMACSecret:  "secret",
		Issuer:      "issuer",
		Audience:    "aud",
		TenantClaim: "tenant",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func TestJWTValid(t *testing.T) {
	e := jwtEngine(t)
	token := signHS256(t, "secret", map[string]any{
		"iss": "issuer", "aud": "aud", "tenant": "acme", "exp": time.Now().Add(time.Hour).Unix(),
	})
	tenant, err := e.AuthenticateJWT(token)
	if err != nil || tenant != "acme" {
		t.Fatalf("tenant=%q err=%v", tenant, err)
	}
}

func TestJWTExpired(t *testing.T) {
	e := jwtEngine(t)
	token := signHS256(t, "secret", map[string]any{
		"iss": "issuer", "aud": "aud", "tenant": "acme", "exp": time.Now().Add(-time.Hour).Unix(),
	})
	if _, err := e.AuthenticateJWT(token); err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestJWTAudience(t *testing.T) {
	e := jwtEngine(t)
	token := signHS256(t, "secret", map[string]any{
		"iss": "issuer", "aud": "other", "tenant": "acme", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := e.AuthenticateJWT(token); err == nil {
		t.Fatal("expected audience error")
	}
}

func TestJWTSignature(t *testing.T) {
	e := jwtEngine(t)
	token := signHS256(t, "wrong", map[string]any{
		"iss": "issuer", "aud": "aud", "tenant": "acme", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := e.AuthenticateJWT(token); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestAuthenticateBearerJWT(t *testing.T) {
	e := jwtEngine(t)
	token := signHS256(t, "secret", map[string]any{
		"iss": "issuer", "aud": "aud", "tenant": "acme", "exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	tenant, err := e.Authenticate(req)
	if err != nil || tenant != "acme" {
		t.Fatalf("tenant=%q err=%v", tenant, err)
	}
}
