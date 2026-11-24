// Command stdio-bridge exposes a newline-delimited JSON-RPC stdio MCP server
// as an HTTP legacy MCP endpoint for Envoy's mcp_router.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dual-era-mcp-gateway/internal/stdio"
)

func main() {
	addr := flag.String("addr", ":18081", "HTTP listen address")
	command := flag.String("command", "", "stdio MCP executable (required)")
	args := flag.String("args", "", "comma-separated executable arguments")
	flag.Parse()
	if *command == "" {
		log.Fatal("-command is required")
	}
	var argv []string
	if strings.TrimSpace(*args) != "" {
		argv = strings.Split(*args, ",")
	}
	bridge, err := stdio.Start(*command, argv...)
	if err != nil {
		log.Fatalf("start stdio child: %v", err)
	}
	defer bridge.Close()
	initCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	initResult, err := bridge.Initialize(initCtx)
	cancel()
	if err != nil {
		log.Fatalf("initialize stdio child: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           stdio.NewHTTPHandler(bridge, initResult),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	log.Printf("stdio bridge on %s -> %s", *addr, *command)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
