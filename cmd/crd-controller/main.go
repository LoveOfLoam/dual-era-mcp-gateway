// Command crd-controller lists/watches McpSource and McpConsumer CRDs and posts
// the compiled config to the control plane. It deliberately uses a small
// dependency-free Kubernetes REST client instead of client-go.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dual-era-mcp-gateway/internal/controller"
	"dual-era-mcp-gateway/internal/kube"
)

func main() {
	api := flag.String("api", envOr("KUBERNETES_API", "https://kubernetes.default.svc"), "Kubernetes API base URL")
	namespace := flag.String("namespace", envOr("POD_NAMESPACE", "default"), "namespace to watch")
	tokenFile := flag.String("token-file", "/var/run/secrets/kubernetes.io/serviceaccount/token", "service account token file")
	controlplane := flag.String("controlplane", "http://controlplane:18080", "control-plane base URL")
	insecure := flag.Bool("insecure", false, "skip Kubernetes API TLS verification (test only)")
	once := flag.Bool("once", false, "reconcile once and exit")
	flag.Parse()

	client, err := kube.NewTokenFileClient(*api, *namespace, *tokenFile)
	if err != nil {
		log.Fatalf("kube client: %v", err)
	}
	if *insecure {
		client.HTTP = &http.Client{Timeout: 40 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	watcher := &kube.Watcher{
		Client:  client,
		Applier: controller.HTTPApplier{BaseURL: *controlplane},
	}
	if *once {
		_, _, err := watcher.ReconcileOnce(ctx)
		if err != nil {
			log.Fatalf("reconcile: %v", err)
		}
		log.Printf("reconciled once from %s/%s", *api, *namespace)
		return
	}
	log.Printf("watching %s/%s (controlplane %s)", *api, *namespace, *controlplane)
	if err := watcher.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("watch: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
