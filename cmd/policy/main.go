// Command policy serves ext_authz and ratelimit gRPC services for Envoy.
// It is intentionally a small, stateless process; policy state is loaded from
// a JSON file in the MVP.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"dual-era-mcp-gateway/internal/audit"
	"dual-era-mcp-gateway/internal/extauthz"
	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/ratelimit"
)

func main() {
	configPath := flag.String("config", "config/policy.example.json", "policy JSON file")
	authzAddr := flag.String("authz-addr", ":19001", "ext_authz gRPC address")
	rlAddr := flag.String("ratelimit-addr", ":19002", "ratelimit gRPC address")
	flag.Parse()

	engine, err := policy.LoadFile(*configPath)
	if err != nil {
		log.Fatalf("policy: %v", err)
	}

	authzLis, err := net.Listen("tcp", *authzAddr)
	if err != nil {
		log.Fatalf("authz listen: %v", err)
	}
	rlLis, err := net.Listen("tcp", *rlAddr)
	if err != nil {
		log.Fatalf("ratelimit listen: %v", err)
	}

	auditLog := audit.New(os.Stdout)

	authzSrv := grpc.NewServer()
	authv3.RegisterAuthorizationServer(authzSrv, extauthz.New(engine).WithAudit(auditLog))
	healthpb.RegisterHealthServer(authzSrv, health.NewServer())

	rlSrv := grpc.NewServer()
	ratelimitv3.RegisterRateLimitServiceServer(rlSrv, ratelimit.New(engine).WithAudit(auditLog))
	healthpb.RegisterHealthServer(rlSrv, health.NewServer())

	go func() {
		if err := authzSrv.Serve(authzLis); err != nil {
			log.Printf("authz serve: %v", err)
		}
	}()
	go func() {
		if err := rlSrv.Serve(rlLis); err != nil {
			log.Printf("ratelimit serve: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	authzSrv.GracefulStop()
	rlSrv.GracefulStop()
}
