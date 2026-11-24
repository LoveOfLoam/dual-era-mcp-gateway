// Package source defines the config-provider and tool-source specifications.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Type is a tool source type.
type Type string

const (
	TypeMCP  Type = "mcp"
	TypeREST Type = "rest"
)

// ToolSpec describes one tool contributed by a source.
type ToolSpec struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty" yaml:"inputSchema,omitempty"`
	Method      string         `json:"method,omitempty" yaml:"method,omitempty"`
	Path        string         `json:"path,omitempty" yaml:"path,omitempty"`
}

// SchemaJSON returns the input schema as raw JSON.
func (t ToolSpec) SchemaJSON() json.RawMessage {
	if len(t.InputSchema) == 0 {
		return json.RawMessage(`{"type":"object"}`)
	}
	raw, _ := json.Marshal(t.InputSchema)
	return raw
}

// SourceSpec is the normalized source definition produced by a Provider.
type SourceSpec struct {
	Name         string            `json:"name" yaml:"name"`
	Type         Type              `json:"type" yaml:"type"`
	Protocol     string            `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Endpoint     string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	BaseURL      string            `json:"baseURL,omitempty" yaml:"baseURL,omitempty"`
	Prefix       string            `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	OpenAPI      string            `json:"openapi,omitempty" yaml:"openapi,omitempty"`
	Headers      map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	AllowedHosts []string          `json:"allowedHosts,omitempty" yaml:"allowedHosts,omitempty"`
	Tools        []ToolSpec        `json:"tools,omitempty" yaml:"tools,omitempty"`
}

// Provider loads source definitions from a backend (file, K8s, Nacos).
type Provider interface {
	Name() string
	Load(ctx context.Context) ([]SourceSpec, error)
	Watch(ctx context.Context) (<-chan []SourceSpec, error)
}

var nameRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// Validate checks source names, prefixes, and required fields.
func Validate(specs []SourceSpec) error {
	names := map[string]bool{}
	prefixes := map[string]bool{}
	for _, s := range specs {
		if !nameRE.MatchString(s.Name) {
			return fmt.Errorf("source %q: invalid name", s.Name)
		}
		if strings.Contains(s.Name, "__") {
			return fmt.Errorf("source %q: name must not contain __", s.Name)
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate source %q", s.Name)
		}
		names[s.Name] = true
		prefix := s.Prefix
		if prefix == "" {
			prefix = s.Name
		}
		if !nameRE.MatchString(prefix) {
			return fmt.Errorf("source %q: invalid prefix %q", s.Name, prefix)
		}
		if prefixes[prefix] {
			return fmt.Errorf("duplicate prefix %q", prefix)
		}
		prefixes[prefix] = true
		switch s.Type {
		case TypeMCP:
			if s.Endpoint == "" {
				return fmt.Errorf("source %q: mcp requires endpoint", s.Name)
			}
		case TypeREST:
			if s.BaseURL == "" {
				return fmt.Errorf("source %q: rest requires baseURL", s.Name)
			}
			if len(s.Tools) == 0 {
				return fmt.Errorf("source %q: rest requires at least one tool", s.Name)
			}
		default:
			return fmt.Errorf("source %q: unknown type %q", s.Name, s.Type)
		}
		tools := map[string]bool{}
		for _, t := range s.Tools {
			if t.Name == "" {
				return fmt.Errorf("source %q: empty tool name", s.Name)
			}
			if tools[t.Name] {
				return fmt.Errorf("source %q: duplicate tool %q", s.Name, t.Name)
			}
			tools[t.Name] = true
		}
	}
	return nil
}

// StaticProvider loads sources from a YAML file.
type StaticProvider struct {
	name string
	path string
}

// NewStatic creates a static YAML provider.
func NewStatic(name, path string) *StaticProvider {
	return &StaticProvider{name: name, path: path}
}

// Name implements Provider.
func (p *StaticProvider) Name() string { return p.name }

// Load reads and validates the YAML file.
func (p *StaticProvider) Load(context.Context) ([]SourceSpec, error) {
	raw, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p.path, err)
	}
	specs, err := ParseSources(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", p.path, err)
	}
	return specs, nil
}

// ParseSources parses and validates a `sources:` YAML document. Providers share
// this so K8s/Nacos/static all enforce the same invariants.
func ParseSources(raw []byte) ([]SourceSpec, error) {
	var doc struct {
		Sources []SourceSpec `yaml:"sources"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if err := Validate(doc.Sources); err != nil {
		return nil, err
	}
	return doc.Sources, nil
}

// Watch closes when the context is done. File watching is intentionally out of
// scope for the MVP; the control plane can call Load on demand.
func (p *StaticProvider) Watch(ctx context.Context) (<-chan []SourceSpec, error) {
	ch := make(chan []SourceSpec)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}
