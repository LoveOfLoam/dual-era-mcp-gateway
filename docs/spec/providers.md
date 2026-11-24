# Providers

## 1. Interface

```go
type Provider interface {
    Name() string
    Load(ctx context.Context) ([]SourceSpec, error)
    Watch(ctx context.Context) (<-chan []SourceSpec, error)
}
```

## 2. Static provider (MVP)

- Reads `config/sources.yaml`.
- On file change (optional), reloads.
- Validation errors are returned; old config remains active.

## 3. OpenAPI provider (MVP)

- Reads OpenAPI 3.0/3.1 JSON/YAML.
- Maps:
  - `operationId` → tool name (fallback: method + path).
  - `summary`/`description` → tool description.
  - parameters + requestBody → `inputSchema`.
  - servers[0].url → baseURL.
  - security → auth reference (manual mapping).
- Limitations: no full `$ref` resolution in MVP; no callbacks; no complex content types.
- Generated `SourceSpec` type `rest`.

## 4. K8s provider (MVP: manifest; Phase 2: informer)

- Implemented: `K8sManifestProvider` reads a ConfigMap (`data["sources.yaml"]` or `data["sources"]`) or a direct `McpSource`-style `spec.sources`.
- This covers GitOps exports, `kubectl get cm -o yaml`, and test fixtures without client-go.
- Phase 2: live informer/CRD controller; policies as CRDs.
- `Watch` currently closes on context cancellation; an informer will emit snapshots through the same interface.

## 5. Nacos provider (MVP: Config Service; Phase 2: discovery)

- Implemented: `NacosProvider` calls `/nacos/v1/cs/configs` with `dataId`, `group`, `tenant`, parses `sources:`, and validates.
- `Watch` polls every 30s and emits when the parsed snapshot changes.
- Phase 2: Nacos service discovery to resolve `host:port` for native MCP/REST services.

## 6. Reload semantics

- Provider emits a full snapshot of `SourceSpec`.
- Control plane validates; if invalid, keep previous snapshot and emit an error metric.
- Version = SHA-256 of normalized sources + policies.
- Data plane reload is atomic (registry swap).

## 7. Mapping to Higress concepts

| Higress | This project |
|---|---|
| McpBridge registries (Nacos/K8s/static) | Provider |
| mcp-servers config | SourceSpec |
| OpenAPI-to-MCP | OpenAPI provider |
| mcp-router | internal Envoy + registry |

We do not copy Higress CRDs; we keep a smaller `SourceSpec` model.

## 8. Testing

- Static provider load/validate/reload.
- OpenAPI provider generates expected tools/schemas.
- K8s ConfigMap/McpSource manifest load and missing-data rejection.
- Nacos Config Service query/parse/error status.
- Invalid provider output rejected; old snapshot remains.
- Provider interface fake used by control-plane tests.