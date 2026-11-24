// Command sse-shim serves the legacy GET/SSE transport for mcp_router, which
// only accepts POST JSON-RPC.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dual-era-mcp-gateway/internal/sse"
)

func main() {
	addr := flag.String("addr", ":18081", "HTTP listen address")
	upstream := flag.String("upstream", "http://envoy-legacy:10001/mcp", "internal MCP HTTP endpoint")
	flag.Parse()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           sse.New(*upstream).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	log.Printf("sse shim on %s -> %s", *addr, *upstream)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
