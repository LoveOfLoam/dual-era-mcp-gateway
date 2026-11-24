// Package kube is a dependency-free Kubernetes CRD client. It implements the
// list/watch loop for McpSource/McpConsumer without client-go so the gateway
// can ship a small controller image.
package kube

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"dual-era-mcp-gateway/internal/controlplane"
	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/source"
)

const (
	Group     = "gateway.dual-era.io"
	Version   = "v1alpha1"
	SourceRes = "mcpsources"
	ConsumRes = "mcpconsumers"
)

// Client talks to the Kubernetes API server with a bearer token.
type Client struct {
	BaseURL   string
	Namespace string
	Token     string
	HTTP      *http.Client
}

// NewTokenFileClient loads a service-account token once.
func NewTokenFileClient(baseURL, namespace, tokenFile string) (*Client, error) {
	c := &Client{BaseURL: strings.TrimRight(baseURL, "/"), Namespace: namespace, HTTP: &http.Client{Timeout: 40 * time.Second}}
	if tokenFile != "" {
		raw, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, err
		}
		c.Token = strings.TrimSpace(string(raw))
	}
	if c.Namespace == "" {
		c.Namespace = "default"
	}
	return c, nil
}

// Object is the subset of a CRD object the controller needs.
type Object struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   ObjectMeta      `json:"metadata"`
	Spec       json.RawMessage `json:"spec"`
}

// ObjectMeta is the Kubernetes object metadata subset.
type ObjectMeta struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	ResourceVersion string `json:"resourceVersion"`
}

type listResponse struct {
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
	Items []Object `json:"items"`
}

// List returns all objects and the current resourceVersion.
func (c *Client) List(ctx context.Context, resource string) ([]Object, string, error) {
	u := fmt.Sprintf("%s/apis/%s/%s/namespaces/%s/%s", c.BaseURL, Group, Version, url.PathEscape(c.Namespace), resource)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	c.auth(req)
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, "", fmt.Errorf("kube list %s: %d %s", resource, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out listResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	return out.Items, out.Metadata.ResourceVersion, nil
}

// Event is a watch event.
type Event struct {
	Type   string `json:"type"`
	Object Object `json:"object"`
}

// Watch streams events until ctx is canceled or the server closes the stream.
func (c *Client) Watch(ctx context.Context, resource, resourceVersion string) (<-chan Event, error) {
	u := fmt.Sprintf("%s/apis/%s/%s/namespaces/%s/%s?watch=true&timeoutSeconds=30",
		c.BaseURL, Group, Version, url.PathEscape(c.Namespace), resource)
	if resourceVersion != "" {
		u += "&resourceVersion=" + url.QueryEscape(resourceVersion)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("kube watch %s: status %d", resource, resp.StatusCode)
	}
	ch := make(chan Event)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 4<<20)
		for sc.Scan() {
			var ev Event
			if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
				continue
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (c *Client) auth(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// ConfigFromObjects converts CRD specs into a controlplane.Config.
func ConfigFromObjects(sources, consumers []Object) (controlplane.Config, error) {
	var cfg controlplane.Config
	for _, obj := range sources {
		var s source.SourceSpec
		if err := json.Unmarshal(obj.Spec, &s); err != nil {
			return cfg, fmt.Errorf("source %s: %w", obj.Metadata.Name, err)
		}
		cfg.Sources = append(cfg.Sources, s)
	}
	for _, obj := range consumers {
		var c policy.Consumer
		if err := json.Unmarshal(obj.Spec, &c); err != nil {
			return cfg, fmt.Errorf("consumer %s: %w", obj.Metadata.Name, err)
		}
		cfg.Consumers = append(cfg.Consumers, c)
	}
	return cfg, nil
}
