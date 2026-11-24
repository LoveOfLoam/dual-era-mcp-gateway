// Package controller reconciles a directory of McpSource/McpConsumer YAML
// (typically a ConfigMap volume) into the control plane. It does not watch the
// Kubernetes CRD API; that requires an informer/client-go controller.
package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"dual-era-mcp-gateway/internal/controlplane"
	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/source"
)

// Applier applies a compiled config to the control plane.
type Applier interface {
	Apply(ctx context.Context, cfg controlplane.Config) error
}

// HTTPApplier posts to a control plane's /api/v1/apply endpoint.
type HTTPApplier struct {
	BaseURL string
	Client  *http.Client
}

// Apply implements Applier.
func (a HTTPApplier) Apply(ctx context.Context, cfg controlplane.Config) error {
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.BaseURL, "/")+"/api/v1/apply", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("control plane apply status %d", resp.StatusCode)
	}
	return nil
}

// Reconciler polls a directory and applies changed configs.
type Reconciler struct {
	Dir      string
	Applier  Applier
	Interval time.Duration
	lastHash string
}

// Run blocks until ctx is canceled.
func (r *Reconciler) Run(ctx context.Context) error {
	interval := r.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.Reconcile(ctx); err != nil {
			// Keep the previous applied version; surface through the caller log.
			fmt.Printf("controller reconcile: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Reconcile loads the directory and applies it if the content hash changed.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	cfg, err := LoadDir(r.Dir)
	if err != nil {
		return err
	}
	if err := controlplane.Validate(cfg); err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if hash == r.lastHash {
		return nil
	}
	if r.Applier == nil {
		return fmt.Errorf("no applier configured")
	}
	if err := r.Applier.Apply(ctx, cfg); err != nil {
		return err
	}
	r.lastHash = hash
	return nil
}

// LoadDir reads *.yaml/*.yml/*.json McpSource/McpConsumer objects.
func LoadDir(dir string) (controlplane.Config, error) {
	var cfg controlplane.Config
	entries, err := os.ReadDir(dir)
	if err != nil {
		return cfg, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return cfg, err
		}
		docs, err := decodeDocs(raw)
		if err != nil {
			return cfg, fmt.Errorf("%s: %w", name, err)
		}
		for _, doc := range docs {
			if err := appendObject(&cfg, doc); err != nil {
				return cfg, fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return cfg, nil
}

func decodeDocs(raw []byte) ([]map[string]any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var docs []map[string]any
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(doc) > 0 {
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

func appendObject(cfg *controlplane.Config, doc map[string]any) error {
	kind, _ := doc["kind"].(string)
	if kind == "" {
		return nil
	}
	spec, ok := doc["spec"].(map[string]any)
	if !ok {
		return fmt.Errorf("kind %s: missing spec", kind)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	switch kind {
	case "McpSource":
		var s source.SourceSpec
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		cfg.Sources = append(cfg.Sources, s)
	case "McpConsumer":
		var c policy.Consumer
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		cfg.Consumers = append(cfg.Consumers, c)
	case "McpRateLimit":
		var l policy.RateLimit
		if err := json.Unmarshal(raw, &l); err != nil {
			return err
		}
		cfg.RateLimits = append(cfg.RateLimits, l)
	default:
		return fmt.Errorf("unsupported kind %q", kind)
	}
	return nil
}
