package source

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OpenAPIProvider converts an OpenAPI 3.x document into one REST SourceSpec.
// The MVP supports JSON/YAML documents, path/query parameters, and inline
// application/json requestBody schemas. External $refs are not resolved.
type OpenAPIProvider struct {
	name       string
	path       string
	sourceName string
	prefix     string
}

// NewOpenAPI creates an OpenAPI provider.
func NewOpenAPI(name, path, sourceName, prefix string) *OpenAPIProvider {
	if sourceName == "" {
		sourceName = "openapi"
	}
	if prefix == "" {
		prefix = sourceName
	}
	return &OpenAPIProvider{name: name, path: path, sourceName: sourceName, prefix: prefix}
}

// Name implements Provider.
func (p *OpenAPIProvider) Name() string { return p.name }

// Watch closes when the context is done.
func (p *OpenAPIProvider) Watch(ctx context.Context) (<-chan []SourceSpec, error) {
	ch := make(chan []SourceSpec)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

type openAPIDoc struct {
	OpenAPI string `yaml:"openapi"`
	Info    struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths map[string]map[string]openAPIOperation `yaml:"paths"`
}

type openAPIOperation struct {
	OperationID string          `yaml:"operationId"`
	Summary     string          `yaml:"summary"`
	Description string          `yaml:"description"`
	Parameters  []openAPIParam  `yaml:"parameters"`
	RequestBody *openAPIReqBody `yaml:"requestBody"`
}

type openAPIParam struct {
	Name     string         `yaml:"name"`
	In       string         `yaml:"in"`
	Required bool           `yaml:"required"`
	Schema   map[string]any `yaml:"schema"`
}

type openAPIReqBody struct {
	Content map[string]struct {
		Schema map[string]any `yaml:"schema"`
	} `yaml:"content"`
}

// Load converts the OpenAPI document into a SourceSpec.
func (p *OpenAPIProvider) Load(context.Context) ([]SourceSpec, error) {
	raw, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p.path, err)
	}
	var doc openAPIDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p.path, err)
	}
	if len(doc.Servers) == 0 || doc.Servers[0].URL == "" {
		return nil, fmt.Errorf("%s: servers[0].url is required", p.path)
	}

	paths := make([]string, 0, len(doc.Paths))
	for path := range doc.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var tools []ToolSpec
	for _, path := range paths {
		methods := doc.Paths[path]
		names := make([]string, 0, len(methods))
		for method := range methods {
			names = append(names, method)
		}
		sort.Strings(names)
		for _, method := range names {
			op := methods[method]
			switch strings.ToLower(method) {
			case "get", "post", "put", "patch", "delete":
			default:
				continue
			}
			tool := ToolSpec{
				Name:        toolName(op, method, path),
				Description: firstNonEmpty(op.Summary, op.Description),
				InputSchema: inputSchema(op),
				Method:      strings.ToUpper(method),
				Path:        path,
			}
			tools = append(tools, tool)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	specs := []SourceSpec{{
		Name:    p.sourceName,
		Type:    TypeREST,
		BaseURL: doc.Servers[0].URL,
		Prefix:  p.prefix,
		Tools:   tools,
	}}
	if err := Validate(specs); err != nil {
		return nil, err
	}
	return specs, nil
}

func toolName(op openAPIOperation, method, path string) string {
	if op.OperationID != "" {
		return op.OperationID
	}
	s := strings.ToLower(method) + "_" + strings.Trim(path, "/")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func inputSchema(op openAPIOperation) map[string]any {
	props := map[string]any{}
	var required []string
	for _, param := range op.Parameters {
		if param.Name == "" {
			continue
		}
		if param.In != "path" && param.In != "query" {
			continue
		}
		schema := param.Schema
		if len(schema) == 0 {
			schema = map[string]any{"type": "string"}
		}
		props[param.Name] = schema
		if param.Required {
			required = append(required, param.Name)
		}
	}
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content["application/json"]; ok {
			if bodyProps, ok := media.Schema["properties"].(map[string]any); ok {
				for k, v := range bodyProps {
					props[k] = v
				}
			}
			if req, ok := media.Schema["required"].([]any); ok {
				for _, r := range req {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
