package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestStaticProviderLoad(t *testing.T) {
	path := writeFile(t, "sources.yaml", `
sources:
  - name: weather
    type: mcp
    protocol: legacy
    endpoint: http://weather:8080/mcp
    prefix: weather
    tools:
      - name: get_weather
        description: Get weather
        inputSchema:
          type: object
          properties:
            q:
              type: string
  - name: crm
    type: rest
    baseURL: https://crm.example.com
    prefix: crm
    tools:
      - name: get_customer
        description: Get customer
        method: GET
        path: /v1/customers/{id}
        inputSchema:
          type: object
          properties:
            id:
              type: string
`)
	p := NewStatic("static", path)
	specs, err := p.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("specs = %+v", specs)
	}
	if specs[0].Name != "weather" || specs[0].Type != TypeMCP || specs[0].Endpoint == "" {
		t.Fatalf("weather = %+v", specs[0])
	}
	if specs[1].Name != "crm" || specs[1].Type != TypeREST || specs[1].Tools[0].Name != "get_customer" {
		t.Fatalf("crm = %+v", specs[1])
	}
}

func TestValidateRejectsDuplicatePrefix(t *testing.T) {
	err := Validate([]SourceSpec{
		{Name: "a", Type: TypeMCP, Endpoint: "http://a", Prefix: "same"},
		{Name: "b", Type: TypeMCP, Endpoint: "http://b", Prefix: "same"},
	})
	if err == nil {
		t.Fatal("expected duplicate prefix error")
	}
}

func TestValidateRejectsMissingFields(t *testing.T) {
	for _, spec := range []SourceSpec{
		{Name: "a", Type: TypeMCP, Prefix: "a"},
		{Name: "b", Type: TypeREST, BaseURL: "https://b", Prefix: "b"},
		{Name: "c", Type: "unknown", Prefix: "c"},
	} {
		if err := Validate([]SourceSpec{spec}); err == nil {
			t.Fatalf("expected error for %+v", spec)
		}
	}
}

func TestValidateAuth(t *testing.T) {
	base := SourceSpec{Name: "a", Type: TypeMCP, Endpoint: "http://a:8080/mcp", Prefix: "a"}
	badOAuth := base
	badOAuth.Auth = &AuthSpec{Type: "oauth2"}
	if err := Validate([]SourceSpec{badOAuth}); err == nil {
		t.Fatal("expected oauth2 validation error")
	}
	badHMAC := base
	badHMAC.Auth = &AuthSpec{Type: "hmac"}
	if err := Validate([]SourceSpec{badHMAC}); err == nil {
		t.Fatal("expected hmac validation error")
	}
	good := base
	good.Auth = &AuthSpec{Type: "hmac", Secret: "s", Header: "X-Signature"}
	if err := Validate([]SourceSpec{good}); err != nil {
		t.Fatalf("good auth: %v", err)
	}
}

func TestOpenAPIProvider(t *testing.T) {
	path := writeFile(t, "openapi.json", `{
  "openapi": "3.0.0",
  "info": {"title": "CRM", "version": "1"},
  "servers": [{"url": "https://crm.example.com"}],
  "paths": {
    "/v1/customers/{customer_id}": {
      "get": {
        "operationId": "get_customer",
        "summary": "Get customer",
        "parameters": [
          {"name": "customer_id", "in": "path", "required": true, "schema": {"type": "string"}}
        ]
      }
    },
    "/v1/orders": {
      "post": {
        "operationId": "create_order",
        "summary": "Create order",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "sku": {"type": "string"},
                  "qty": {"type": "integer"}
                },
                "required": ["sku", "qty"]
              }
            }
          }
        }
      }
    }
  }
}`)
	p := NewOpenAPI("crm", path, "crm", "crm")
	specs, err := p.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 || specs[0].Type != TypeREST {
		t.Fatalf("specs = %+v", specs)
	}
	if len(specs[0].Tools) != 2 {
		t.Fatalf("tools = %+v", specs[0].Tools)
	}
	names := map[string]bool{}
	for _, tool := range specs[0].Tools {
		names[tool.Name] = true
	}
	if !names["get_customer"] || !names["create_order"] {
		t.Fatalf("names = %+v", names)
	}
}

func TestStaticProviderReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	mustWrite := func(content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mustWrite(`
sources:
  - name: a
    type: mcp
    endpoint: http://a:8080/mcp
    prefix: a
`)
	p := NewStatic("static", path)
	first, err := p.Load(context.Background())
	if err != nil || len(first) != 1 || first[0].Name != "a" {
		t.Fatalf("first load = %+v, %v", first, err)
	}
	mustWrite(`
sources:
  - name: b
    type: rest
    baseURL: https://b.example.com
    prefix: b
    tools:
      - name: ping
`)
	second, err := p.Load(context.Background())
	if err != nil || len(second) != 1 || second[0].Name != "b" {
		t.Fatalf("second load = %+v, %v", second, err)
	}
}

func TestStaticProviderWatchClosesOnContext(t *testing.T) {
	path := writeFile(t, "sources.yaml", `sources: []`)
	p := NewStatic("static", path)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	cancel()
	for range ch {
	}
}
