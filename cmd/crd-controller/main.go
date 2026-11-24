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
	leaderElect := flag.Bool("leader-elect", false, "use a coordination.k8s.io Lease for single-writer leadership")
	leaseName := flag.String("lease-name", "mcp-crd-controller", "leader-election Lease name")
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
	run := func(runCtx context.Context) {
		if *once {
			_, _, err := watcher.ReconcileOnce(runCtx)
			if err != nil {
				log.Printf("reconcile: %v", err)
			}
			return
		}
		if err := watcher.Run(runCtx); err != nil && err != context.Canceled {
			log.Printf("watch: %v", err)
		}
	}
	if *leaderElect && !*once {
		identity, _ := os.Hostname()
		if identity == "" {
			identity = "crd-controller"
		}
		elector := &kube.LeaderElector{Client: client, Name: *leaseName, Identity: identity, TTL: 15 * time.Second}
		log.Printf("leader election enabled (lease %s, identity %s)", *leaseName, identity)
		if err := elector.Run(ctx, run); err != nil && err != context.Canceled {
			log.Fatalf("leader election: %v", err)
		}
		return
	}
	log.Printf("watching %s/%s (controlplane %s)", *api, *namespace, *controlplane)
	run(ctx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
