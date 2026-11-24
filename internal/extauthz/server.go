// Package extauthz implements Envoy's ext_authz Authorization gRPC service for
// API-key authentication, tenant derivation, and tool allowlists.
package extauthz

import (
	"context"
	"net/http"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dual-era-mcp-gateway/internal/policy"
)

// Server implements authv3.AuthorizationServer.
type Server struct {
	authv3.UnimplementedAuthorizationServer
	policy *policy.Engine
}

// New creates an ext_authz server.
func New(engine *policy.Engine) *Server {
	return &Server{policy: engine}
}

// Check authorizes one request. It reads the tool from Mcp-Name (or
// X-Mcp-Name) and derives the tenant only from the credential.
func (s *Server) Check(_ context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	headers := requestHeaders(req)
	key := bearerToken(headers["authorization"])
	if key == "" {
		key = headers["x-api-key"]
	}
	tenant, err := s.policy.AuthenticateKey(key)
	if err != nil {
		return denied(http.StatusUnauthorized, codes.Unauthenticated, "unauthenticated"), nil
	}
	tool := headers["mcp-name"]
	if tool == "" {
		tool = headers["x-mcp-name"]
	}
	if tool != "" {
		if err := s.policy.Authorize(tenant, tool); err != nil {
			return denied(http.StatusForbidden, codes.PermissionDenied, "tool not allowed"), nil
		}
	}
	return allowed(tenant), nil
}

func requestHeaders(req *authv3.CheckRequest) map[string]string {
	if req == nil || req.GetAttributes() == nil || req.GetAttributes().GetRequest() == nil {
		return map[string]string{}
	}
	httpReq := req.GetAttributes().GetRequest().GetHttp()
	if httpReq == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(httpReq.GetHeaders()))
	for k, v := range httpReq.GetHeaders() {
		out[strings.ToLower(k)] = v
	}
	return out
}

func allowed(tenant string) *authv3.CheckResponse {
	headers := []*corev3.HeaderValueOption{
		{Header: &corev3.HeaderValue{Key: "x-tenant-id", Value: tenant}},
		{Header: &corev3.HeaderValue{Key: "x-consumer", Value: tenant}},
		{Header: &corev3.HeaderValue{Key: "x-authz-decision", Value: "allow"}},
	}
	return &authv3.CheckResponse{
		Status:       status.New(codes.OK, "").Proto(),
		HttpResponse: &authv3.CheckResponse_OkResponse{OkResponse: &authv3.OkHttpResponse{Headers: headers, HeadersToRemove: []string{"authorization", "x-api-key", "x-tenant-id"}}},
	}
}

func denied(httpCode int, code codes.Code, message string) *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: status.New(code, message).Proto(),
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &envoytype.HttpStatus{Code: envoytype.StatusCode(httpCode)},
				Body:   `{"error":"` + message + `"}`,
			},
		},
	}
}

func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
