# Tool Sources

## 1. Two layers

- **Provider**: where source definitions come from (static YAML, OpenAPI, K8s, Nacos).
- **ToolSource**: how a source exposes tools (native MCP, REST-to-MCP).

They are orthogonal. A Nacos provider can produce native MCP or REST sources.

## 2. SourceSpec

```yaml
sources:
  - name: weather
    type: mcp
    protocol: legacy
    endpoint: http://weather:8080/mcp
    prefix: weather
    tools:
      include: ["get_weather", "list_cities"]

  - name: crm
    type: rest
    baseURL: https://crm.example.com
    prefix: crm
    openapi: ./openapi/crm.yaml
    auth:
      secretRef: crm-auth
```

Validation:

- `name` unique, `[a-z0-9-]{1,32}`, no `__`.
- `type`: `mcp` | `rest`.
- `mcp` requires endpoint; `rest` requires baseURL + (openapi or tools).
- `prefix` unique.
- Tool names after prefix must not collide.

## 3. ToolSource interface

```go
type ToolSource interface {
    Name() string
    ListTools(ctx context.Context) ([]Tool, error)
    Call(ctx context.Context, tool string, args json.RawMessage) (Result, error)
    Health(ctx context.Context) error
}
```

## 4. Native MCP source

- Legacy: HTTP JSON-RPC; `initialize` + `notifications/initialized`; session reuse; `tools/list`/`tools/call`.
- Modern: direct MCP client (Phase 2).
- Normalized to internal MCP backend for legacy Envoy `mcp_router`.
- Errors: connection, timeout, JSON-RPC error, invalid response.

## 5. REST source

### Simple REST (MVP)

- Use Envoy `mcp_json_rest_bridge` on an internal listener.
- It becomes an MCP backend for `mcp_router`.
- Supports 2026-07-28 `_meta`, `server/discover`, `ttlMs/cacheScope`, `Mcp-Param-*`.
- Map: HTTP method/path/query/body ↔ tool arguments.

### Complex REST (Phase 2)

Implement `restAdapter` as a Go MCP backend:

- OAuth client credentials / token exchange
- HMAC signing
- multipart/binary
- long-running jobs
- complex pagination
- custom error mapping

## 6. Normalization rules

- Tool name: `<prefix>__<tool>`.
- Sort: lexicographic by tool name.
- Collision: reject config.
- Single source: still prefix; do not rely on Envoy router's single-backend behavior.
- Partial failure: healthy sources remain; stale source marked; `tools/list` excludes stale or marks it (configurable).

## 7. Mixed catalog example

| Source | Type | Tool | Exposed name |
|---|---|---|---|
| weather | native MCP | get_weather | weather__get_weather |
| orders | native MCP | create_order | orders__create_order |
| crm | REST | get_customer | crm__get_customer |

## 8. Testing

- T4: native + REST in one catalog.
- T4: source failure does not hide healthy source.
- T5: provider reload adds/removes source.
- T5: collision rejected.
- T6: single-source prefix stable.