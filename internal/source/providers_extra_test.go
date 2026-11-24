package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestK8sConfigMapProvider(t *testing.T) {
	path := writeFile(t, "configmap.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: mcp-sources
data:
  sources.yaml: |
    sources:
      - name: weather
        type: mcp
        endpoint: http://weather:8080/mcp
        prefix: weather
`)
	p := NewK8sManifest("k8s", path)
	specs, err := p.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != "weather" {
		t.Fatalf("specs = %+v", specs)
	}
}

func TestK8sDirectMcpSource(t *testing.T) {
	path := writeFile(t, "mcpsource.yaml", `
kind: McpSource
spec:
  sources:
    - name: crm
      type: rest
      baseURL: https://crm.example.com
      prefix: crm
      tools:
        - name: ping
`)
	p := NewK8sManifest("k8s", path)
	specs, err := p.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 || specs[0].Type != TypeREST {
		t.Fatalf("specs = %+v", specs)
	}
}

func TestK8sMissingData(t *testing.T) {
	path := writeFile(t, "empty.yaml", `kind: ConfigMap`)
	p := NewK8sManifest("k8s", path)
	if _, err := p.Load(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNacosProvider(t *testing.T) {
	var gotDataID, gotGroup, gotTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v1/cs/configs" {
			t.Errorf("path = %s", r.URL.Path)
		}
		gotDataID = r.URL.Query().Get("dataId")
		gotGroup = r.URL.Query().Get("group")
		gotTenant = r.URL.Query().Get("tenant")
		_, _ = w.Write([]byte(`
sources:
  - name: weather
    type: mcp
    endpoint: http://weather:8080/mcp
    prefix: weather
`))
	}))
	defer srv.Close()

	p := NewNacos("nacos", srv.URL, "sources.yaml", "DEFAULT_GROUP", "tenant-a")
	specs, err := p.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != "weather" {
		t.Fatalf("specs = %+v", specs)
	}
	if gotDataID != "sources.yaml" || gotGroup != "DEFAULT_GROUP" || gotTenant != "tenant-a" {
		t.Fatalf("query = %s/%s/%s", gotDataID, gotGroup, gotTenant)
	}
}

func TestNacosProviderErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewNacos("nacos", srv.URL, "missing", "DEFAULT_GROUP", "")
	if _, err := p.Load(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
