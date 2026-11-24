package extauthz

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc/codes"

	"dual-era-mcp-gateway/internal/audit"
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
	})
	if err != nil {
		t.Fatalf("policy.New: %v", err)
	}
	return e
}

func checkRequest(headers map[string]string) *authv3.CheckRequest {
	return &authv3.CheckRequest{Attributes: &authv3.AttributeContext{
		Request: &authv3.AttributeContext_Request{
			Http: &authv3.AttributeContext_HttpRequest{Headers: headers},
		},
	}}
}

func TestCheckAllowed(t *testing.T) {
	s := New(testPolicy(t))
	resp, err := s.Check(context.Background(), checkRequest(map[string]string{
		"authorization": "Bearer acme-key",
		"mcp-name":      "weather__get_weather",
		"x-tenant-id":   "spoofed",
	}))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Fatalf("status = %+v", resp.GetStatus())
	}
	ok := resp.GetOkResponse()
	if ok == nil {
		t.Fatal("missing OkResponse")
	}
	foundTenant := false
	for _, h := range ok.GetHeaders() {
		if h.GetHeader().GetKey() == "x-tenant-id" && h.GetHeader().GetValue() == "acme" {
			foundTenant = true
		}
	}
	if !foundTenant {
		t.Fatalf("tenant header missing: %+v", ok.GetHeaders())
	}
	for _, h := range ok.GetHeadersToRemove() {
		if h == "x-tenant-id" {
			t.Fatal("x-tenant-id must not be removed after being injected")
		}
	}
}

func TestCheckUnauthenticated(t *testing.T) {
	s := New(testPolicy(t))
	resp, _ := s.Check(context.Background(), checkRequest(map[string]string{}))
	if resp.GetStatus().GetCode() != int32(codes.Unauthenticated) {
		t.Fatalf("status = %+v", resp.GetStatus())
	}
	if resp.GetDeniedResponse().GetStatus().GetCode() != http.StatusUnauthorized {
		t.Fatalf("http status = %+v", resp.GetDeniedResponse().GetStatus())
	}
}

func TestCheckForbidden(t *testing.T) {
	s := New(testPolicy(t))
	resp, _ := s.Check(context.Background(), checkRequest(map[string]string{
		"authorization": "Bearer acme-key",
		"mcp-name":      "crm__create_order",
	}))
	if resp.GetStatus().GetCode() != int32(codes.PermissionDenied) {
		t.Fatalf("status = %+v", resp.GetStatus())
	}
	if resp.GetDeniedResponse().GetStatus().GetCode() != http.StatusForbidden {
		t.Fatalf("http status = %+v", resp.GetDeniedResponse().GetStatus())
	}
}

func TestAuditEvent(t *testing.T) {
	var buf bytes.Buffer
	s := New(testPolicy(t)).WithAudit(audit.New(&buf))
	_, _ = s.Check(context.Background(), checkRequest(map[string]string{
		"authorization": "Bearer acme-key",
		"mcp-name":      "weather__get_weather",
		"mcp-method":    "tools/call",
	}))
	var event map[string]any
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("audit json: %v", err)
	}
	if event["decision"] != "allow" || event["tenant"] != "acme" {
		t.Fatalf("event = %+v", event)
	}
}

func TestCheckListWithoutTool(t *testing.T) {
	s := New(testPolicy(t))
	resp, _ := s.Check(context.Background(), checkRequest(map[string]string{"authorization": "Bearer acme-key"}))
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Fatalf("status = %+v", resp.GetStatus())
	}
}
