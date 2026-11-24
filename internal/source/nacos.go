package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NacosProvider fetches a `sources:` YAML document from Nacos Config Service.
// Service discovery is intentionally not mixed into this provider; a future
// Nacos discovery adapter can rewrite endpoints before Load returns.
type NacosProvider struct {
	name     string
	baseURL  string
	dataID   string
	group    string
	tenant   string
	interval time.Duration
	client   *http.Client
}

// NewNacos creates a Nacos config provider.
func NewNacos(name, baseURL, dataID, group, tenant string) *NacosProvider {
	if group == "" {
		group = "DEFAULT_GROUP"
	}
	return &NacosProvider{
		name:     name,
		baseURL:  strings.TrimRight(baseURL, "/"),
		dataID:   dataID,
		group:    group,
		tenant:   tenant,
		interval: 30 * time.Second,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Name implements Provider.
func (p *NacosProvider) Name() string { return p.name }

// Load fetches the config and validates it with the shared source validator.
func (p *NacosProvider) Load(ctx context.Context) ([]SourceSpec, error) {
	endpoint, err := url.Parse(p.baseURL + "/nacos/v1/cs/configs")
	if err != nil {
		return nil, fmt.Errorf("nacos baseURL: %w", err)
	}
	q := endpoint.Query()
	q.Set("dataId", p.dataID)
	q.Set("group", p.group)
	if p.tenant != "" {
		q.Set("tenant", p.tenant)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nacos get: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nacos get: status %d: %s", resp.StatusCode, string(raw))
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("nacos get: empty config %s/%s", p.group, p.dataID)
	}
	specs, err := ParseSources(raw)
	if err != nil {
		return nil, fmt.Errorf("nacos config %s/%s: %w", p.group, p.dataID, err)
	}
	return specs, nil
}

// Watch polls Nacos and emits a snapshot when the parsed sources change.
func (p *NacosProvider) Watch(ctx context.Context) (<-chan []SourceSpec, error) {
	ch := make(chan []SourceSpec)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		var last string
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				specs, err := p.Load(ctx)
				if err != nil {
					continue
				}
				sig := fmt.Sprintf("%+v", specs)
				if sig == last {
					continue
				}
				last = sig
				select {
				case ch <- specs:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}
