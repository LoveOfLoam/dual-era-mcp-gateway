// Command gateway runs the B1 dual-era endpoints without Envoy for local tests
// and demos. Production uses the generated Envoy config in deploy/envoy.
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

	"dual-era-mcp-gateway/internal/aggregator"
	"dual-era-mcp-gateway/internal/edge"
	"dual-era-mcp-gateway/internal/facade"
	"dual-era-mcp-gateway/internal/legacy"
	"dual-era-mcp-gateway/internal/legacyendpoint"
	"dual-era-mcp-gateway/internal/mcp"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	legacyURLs := flag.String("legacy", envOr("LEGACY_URLS", "weather=http://127.0.0.1:9001/mcp"), "comma-separated name=url legacy MCP upstreams")
	flag.Parse()

	backends := parseBackends(*legacyURLs)
	router, err := aggregator.New(backends)
	if err != nil {
		log.Fatalf("aggregator: %v", err)
	}
	legacyHandler := legacyendpoint.New(legacyendpoint.Config{
		ServerInfo:      legacyendpoint.ServerInfo{Name: "dual-era-legacy", Version: "0.1.0"},
		ProtocolVersion: mcp.Version20250618,
		SessionID:       "sess-local",
	}, router)
	modernHandler := facade.New(facade.Config{
		ServerInfo:        facade.ServerInfo{Name: "dual-era-modern", Version: "0.1.0"},
		SupportedVersions: []string{mcp.Version20260728},
		TTLMs:             60000,
		CacheScope:        "public",
		MaxBodyBytes:      1 << 20,
	}, router)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           edge.New(legacyHandler, modernHandler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("dual-era gateway listening on %s (legacy /mcp, modern /modern/mcp)", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func parseBackends(spec string) []aggregator.Backend {
	var out []aggregator.Backend
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, url, ok := strings.Cut(part, "=")
		if !ok || name == "" || url == "" {
			log.Printf("skip invalid legacy upstream %q (want name=url)", part)
			continue
		}
		out = append(out, aggregator.Backend{Name: name, Client: legacy.NewHTTPClient(url)})
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
