// Command admission serves the ValidatingAdmissionWebhook for McpSource and
// McpConsumer. Use -cert/-key in a cluster; plain HTTP is for local tests only.
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

	"dual-era-mcp-gateway/internal/admission"
)

func main() {
	addr := flag.String("addr", ":18443", "listen address")
	cert := flag.String("cert", "", "TLS certificate file (required in a cluster)")
	key := flag.String("key", "", "TLS private key file")
	flag.Parse()

	mux := http.NewServeMux()
	mux.Handle("/validate", admission.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		var err error
		if *cert != "" && *key != "" {
			err = srv.ListenAndServeTLS(*cert, *key)
		} else {
			log.Printf("admission webhook serving plain HTTP; a real webhook requires TLS")
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	log.Printf("admission webhook on %s", *addr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
