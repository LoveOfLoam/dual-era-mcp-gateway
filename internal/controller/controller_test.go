package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestReconcileOnlyOnChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sources.yaml")
	write(t, file, `
kind: McpSource
spec:
  name: weather
  type: mcp
  endpoint: http://weather:9001/mcp
  prefix: weather
`)
	applier := &fakeApplier{}
	r := &Reconciler{Dir: dir, Applier: applier}
	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("first: %v", err)
	}
	if applier.calls != 1 || len(applier.last.Sources) != 1 {
		t.Fatalf("calls=%d cfg=%+v", applier.calls, applier.last)
	}
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("second: %v", err)
	}
	if applier.calls != 1 {
		t.Fatalf("unchanged should not apply, calls=%d", applier.calls)
	}
	write(t, file, `
kind: McpSource
spec:
  name: weather
  type: mcp
  endpoint: http://weather2:9001/mcp
  prefix: weather
`)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("changed: %v", err)
	}
	if applier.calls != 2 {
		t.Fatalf("changed should apply, calls=%d", applier.calls)
	}
}

func TestLoadMultipleKinds(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "all.yaml"), `
kind: McpSource
spec: {name: weather, type: mcp, endpoint: "http://weather:9001/mcp", prefix: weather}
---
kind: McpConsumer
spec:
  name: acme
  tenant: acme
  apiKeys:
    - name: prod
      sha256: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
  allow:
    - source: weather
      tools: ["*"]
`)
	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(cfg.Sources) != 1 || len(cfg.Consumers) != 1 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Consumers[0].Tenant != "acme" {
		t.Fatalf("tenant = %q", cfg.Consumers[0].Tenant)
	}
}

func TestHTTPApplier(t *testing.T) {
	var gotPath string
	var got controlplane.Config
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := (HTTPApplier{BaseURL: srv.URL}).Apply(context.Background(), controlplane.Config{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if gotPath != "/api/v1/apply" || got.Sources != nil {
		t.Fatalf("path=%q cfg=%+v", gotPath, got)
	}
}

func TestUnsupportedKind(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "bad.yaml"), "kind: Other\nspec: {}\n")
	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("err = %v", err)
	}
}
