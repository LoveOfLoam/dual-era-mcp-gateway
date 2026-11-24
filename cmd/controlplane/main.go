// Command controlplane exposes a small versioned configuration API and a
// generated Envoy config. It is a local MVP, not a Kubernetes controller.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dual-era-mcp-gateway/internal/controlplane"
	"dual-era-mcp-gateway/internal/envoy"
	"dual-era-mcp-gateway/internal/source"
	"dual-era-mcp-gateway/internal/xds"
)

func main() {
	addr := flag.String("addr", ":18080", "listen address")
	xdsAddr := flag.String("xds-addr", ":18000", "xDS gRPC listen address")
	configPath := flag.String("config", "", "optional controlplane.Config JSON file")
	testInvalid := flag.Bool("test-invalid", false, "enable /api/v1/inject-invalid (T8 NACK harness only)")
	flag.Parse()

	cfg := demoConfig()
	if *configPath != "" {
		raw, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			log.Fatalf("config: %v", err)
		}
	}
	mgr, err := controlplane.New(cfg)
	if err != nil {
		log.Fatalf("controlplane: %v", err)
	}
	xdsSrv := xds.New("dual-era")
	if err := xdsSrv.Apply(context.Background(), mgr.Current().Version, cfg); err != nil {
		log.Fatalf("xds apply: %v", err)
	}
	actualXDS, err := xdsSrv.Start(*xdsAddr)
	if err != nil {
		log.Fatalf("xds start: %v", err)
	}
	defer xdsSrv.Stop()
	log.Printf("xDS ADS listening on %s", actualXDS)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		cur := mgr.Current()
		writeJSON(w, http.StatusOK, map[string]any{
			"version":     cur.Version,
			"created":     cur.CreatedAt,
			"sources":     len(cur.Sources),
			"xds_version": xdsSrv.CurrentVersion(),
		})
	})
	mux.HandleFunc("/api/v1/config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mgr.Current().Config)
	})
	mux.HandleFunc("/api/v1/revisions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mgr.History())
	})
	mux.HandleFunc("/api/v1/envoy", func(w http.ResponseWriter, _ *http.Request) {
		out, err := envoy.Generate(envoy.Config{Sources: mgr.Current().Sources})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(out))
	})
	mux.HandleFunc("/api/v1/dry-run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var next controlplane.Config
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&next); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := controlplane.Validate(next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, controlplane.DiffConfigs(mgr.Current().Config, next))
	})

	mux.HandleFunc("/api/v1/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var next controlplane.Config
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&next); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		snap, err := mgr.ApplyWithMeta(next, r.Header.Get("X-Actor"), r.Header.Get("X-Reason"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := xdsSrv.Apply(context.Background(), snap.Version, snap.Config); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"version": snap.Version})
	})
	if *testInvalid {
		mux.HandleFunc("/api/v1/inject-invalid", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			version := fmt.Sprintf("invalid-%d", time.Now().UnixNano())
			if err := xdsSrv.InjectInvalid(r.Context(), version); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"version": version})
		})
	}
	mux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mgr.Audit())
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		applies, rollbacks := mgr.Counts()
		acks, nacks := xdsSrv.ACKs().Counts()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, `# HELP mcp_controlplane_apply_total Successful config applies.
# TYPE mcp_controlplane_apply_total counter
mcp_controlplane_apply_total %d
# HELP mcp_controlplane_rollback_total Successful config rollbacks.
# TYPE mcp_controlplane_rollback_total counter
mcp_controlplane_rollback_total %d
# HELP mcp_controlplane_xds_ack_total Data-plane ACK events.
# TYPE mcp_controlplane_xds_ack_total counter
mcp_controlplane_xds_ack_total %d
# HELP mcp_controlplane_xds_nack_total Data-plane NACK events.
# TYPE mcp_controlplane_xds_nack_total counter
mcp_controlplane_xds_nack_total %d
`, applies, rollbacks, acks, nacks)
	})

	mux.HandleFunc("/api/v1/rollback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		snap, err := mgr.RollbackWithMeta(body.Version, r.Header.Get("X-Actor"), r.Header.Get("X-Reason"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := xdsSrv.Apply(context.Background(), snap.Version, snap.Config); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"version": snap.Version})
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		xdsSrv.Stop()
	}()
	log.Printf("controlplane listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func demoConfig() controlplane.Config {
	return controlplane.Config{
		Sources: []source.SourceSpec{
			{Name: "weather", Type: source.TypeMCP, Protocol: "legacy", Endpoint: "http://127.0.0.1:9001/mcp", Prefix: "weather"},
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
