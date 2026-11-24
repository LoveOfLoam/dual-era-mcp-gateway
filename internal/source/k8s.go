package source

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// K8sManifestProvider imports source definitions from a Kubernetes-style YAML
// manifest. It deliberately does not depend on client-go: the same parser can
// be fed a ConfigMap exported by `kubectl get cm -o yaml`, a GitOps file, or a
// test fixture. A live informer can wrap this provider later.
type K8sManifestProvider struct {
	name string
	path string
}

// NewK8sManifest creates a manifest-backed provider.
func NewK8sManifest(name, path string) *K8sManifestProvider {
	return &K8sManifestProvider{name: name, path: path}
}

// Name implements Provider.
func (p *K8sManifestProvider) Name() string { return p.name }

// Load reads a ConfigMap or a direct McpSource-style document.
func (p *K8sManifestProvider) Load(context.Context) ([]SourceSpec, error) {
	raw, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p.path, err)
	}
	var manifest struct {
		Kind string            `yaml:"kind"`
		Data map[string]string `yaml:"data"`
		Spec struct {
			Sources []SourceSpec `yaml:"sources"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p.path, err)
	}

	// ConfigMap: data["sources.yaml"] or data["sources"].
	for _, key := range []string{"sources.yaml", "sources"} {
		if body, ok := manifest.Data[key]; ok && body != "" {
			specs, err := ParseSources([]byte(body))
			if err != nil {
				return nil, fmt.Errorf("%s data[%q]: %w", p.path, key, err)
			}
			return specs, nil
		}
	}
	// Direct McpSource-like document.
	if len(manifest.Spec.Sources) > 0 {
		if err := Validate(manifest.Spec.Sources); err != nil {
			return nil, err
		}
		return manifest.Spec.Sources, nil
	}
	return nil, fmt.Errorf("%s: expected ConfigMap data[sources.yaml] or spec.sources", p.path)
}

// Watch closes when the context is done. A client-go informer can emit new
// snapshots through the same Provider interface later.
func (p *K8sManifestProvider) Watch(ctx context.Context) (<-chan []SourceSpec, error) {
	ch := make(chan []SourceSpec)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}
