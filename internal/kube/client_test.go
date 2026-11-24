package kube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dual-era-mcp-gateway/internal/controlplane"
)

type fakeApplier struct {
	calls int
	last  controlplane.Config
}

func (f *fakeApplier) Apply(_ context.Context, cfg controlplane.Config) error {
	f.calls++
	f.last = cfg
	return nil
}

func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/apis/gateway.dual-era.io/v1alpha1/namespaces/default/mcpsources":
			_, _ = w.Write([]byte(`{"metadata":{"resourceVersion":"10"},"items":[{"metadata":{"name":"weather"},"spec":{"name":"weather","type":"mcp","endpoint":"http://weather:9001/mcp","prefix":"weather"}}]}`))
		case r.URL.Path == "/apis/gateway.dual-era.io/v1alpha1/namespaces/default/mcpconsumers":
			_, _ = w.Write([]byte(`{"metadata":{"resourceVersion":"11"},"items":[{"metadata":{"name":"acme"},"spec":{"tenant":"acme","apiKeys":[{"name":"prod","sha256":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}],"allow":[{"source":"*","tools":["*"]}]}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestReconcileOnce(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	applier := &fakeApplier{}
	w := &Watcher{Client: &Client{BaseURL: srv.URL, Namespace: "default"}, Applier: applier}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sourceRV, consumerRV, err := w.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if sourceRV != "10" || consumerRV != "11" {
		t.Fatalf("rv = %q/%q", sourceRV, consumerRV)
	}
	if applier.calls != 1 || len(applier.last.Sources) != 1 || len(applier.last.Consumers) != 1 {
		t.Fatalf("applied = %+v calls=%d", applier.last, applier.calls)
	}
	if _, _, err := w.ReconcileOnce(ctx); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if applier.calls != 1 {
		t.Fatalf("unchanged config applied again: %d", applier.calls)
	}
}

func TestConfigFromObjects(t *testing.T) {
	src := Object{Metadata: ObjectMeta{Name: "w"}, Spec: json.RawMessage(`{"name":"weather","type":"mcp","endpoint":"http://w:1/mcp","prefix":"weather"}`)}
	con := Object{Metadata: ObjectMeta{Name: "acme"}, Spec: json.RawMessage(`{"tenant":"acme","apiKeys":[{"name":"prod","sha256":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}],"allow":[{"source":"*","tools":["*"]}]}`)}
	cfg, err := ConfigFromObjects([]Object{src}, []Object{con})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Name != "weather" || len(cfg.Consumers) != 1 {
		t.Fatalf("cfg = %+v", cfg)
	}
}
