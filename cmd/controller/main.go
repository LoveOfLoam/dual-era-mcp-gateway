// Command controller reconciles a directory of McpSource/McpConsumer YAML
// (typically a ConfigMap volume) to the control plane. It is not a CRD
// informer; see deploy/controller/README.md for that boundary.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"

	"dual-era-mcp-gateway/internal/controller"
)

func main() {
	dir := flag.String("dir", "/config", "directory containing McpSource/McpConsumer YAML")
	cp := flag.String("controlplane", "http://127.0.0.1:18080", "control plane base URL")
	interval := flag.Duration("interval", 2*time.Second, "poll interval")
	once := flag.Bool("once", false, "reconcile once and exit")
	flag.Parse()

	r := &controller.Reconciler{
		Dir:      *dir,
		Applier:  controller.HTTPApplier{BaseURL: *cp},
		Interval: *interval,
	}
	if *once {
		if err := r.Reconcile(context.Background()); err != nil {
			log.Fatalf("reconcile: %v", err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := r.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("controller: %v", err)
	}
}
