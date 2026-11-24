// Package registry normalizes tools from multiple sources into one catalog.
package registry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Tool is one MCP tool after source normalization.
type Tool struct {
	Name         string          `json:"name"`
	OriginalName string          `json:"originalName,omitempty"`
	Source       string          `json:"source,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
}

// Catalog is an immutable-by-convention tool catalog.
type Catalog struct {
	tools map[string]Tool
}

// Prefix returns the normalized tool name.
func Prefix(source, tool string) string {
	return source + "__" + tool
}

// Merge prefixes, sorts, and validates tools from sources.
func Merge(sources map[string][]Tool) (*Catalog, error) {
	c := &Catalog{tools: make(map[string]Tool)}
	for source, tools := range sources {
		if source == "" || strings.Contains(source, "__") {
			return nil, fmt.Errorf("invalid source name %q: must be non-empty and must not contain __", source)
		}
		for _, t := range tools {
			if t.OriginalName == "" {
				t.OriginalName = t.Name
			}
			if t.Name == "" {
				return nil, fmt.Errorf("source %q: tool name is required", source)
			}
			t.Source = source
			t.Name = Prefix(source, t.OriginalName)
			if _, exists := c.tools[t.Name]; exists {
				return nil, fmt.Errorf("tool collision: %s", t.Name)
			}
			c.tools[t.Name] = t
		}
	}
	return c, nil
}

// List returns tools sorted by name.
func (c *Catalog) List() []Tool {
	out := make([]Tool, 0, len(c.tools))
	for _, t := range c.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Resolve returns the tool by normalized name.
func (c *Catalog) Resolve(name string) (Tool, bool) {
	t, ok := c.tools[name]
	return t, ok
}

// Len returns the number of tools.
func (c *Catalog) Len() int { return len(c.tools) }
