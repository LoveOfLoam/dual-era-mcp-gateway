# Dual-era MCP Gateway — B1 Design

- Date: 2026-11-24
- Status: approved for implementation
- Repo: `dual-era-mcp-gateway`
- Scope: B1 versioned endpoints, native MCP + REST tool sources, static/OpenAPI providers

## 1. Problem

A single MCP endpoint cannot cleanly serve both 2025-xx legacy clients and 2026-07-28 modern clients with the current Envoy filters:

- `mcp_router` is legacy-only and terminal.
- `mcp_json_rest_bridge` supports 2026-07-28 but cannot coexist with terminal `mcp_router` on one listener.
- Path C (legacy public router + internal REST bridge) gives a unified legacy catalog, but a modern client cannot pass `server/discover` at the public router.

Therefore B1 uses **versioned endpoints**:

- `/mcp` → legacy Envoy `mcp_router`.
- `/modern/mcp` → Go modern facade.

## 2. Architecture

```
Client
  │ HTTPS
  ▼
Public Envoy :8443
  ├─ /modern/mcp ──▶ Go modern facade ──▶ LegacyClient ──▶ Internal Envoy :10001
  │                                                          └─ mcp_router
  │                                                               ├─ native MCP A
  │                                                               ├─ native MCP B
  │                                                               └─ REST bridge
  └─ /mcp ─────────▶ Internal Envoy :10001
```

- Public Envoy: TLS, auth, rate limit, path routing. No terminal `mcp_router`.
- Internal Envoy: `mcp_router` + `mcp_multicluster`; owns legacy aggregation.
- Modern facade: implements 2026-07-28; calls `LegacyClient`; has no direct coupling to Envoy.
- REST bridge: simple JSON REST via `mcp_json_rest_bridge`; complex REST via Go adapter (interface only in MVP).

## 3. Tool sources

Two orthogonal layers:

### 3.1 Config providers

```go
type Provider interface {
    Load(ctx context.Context) ([]SourceSpec, error)
    Watch(ctx context.Context) (<-chan []SourceSpec, error)
}
```

MVP:

- `static`: YAML file.
- `openapi`: OpenAPI 3.0/3.1 file → REST tools.

Phase 2:

- `k8s`: ConfigMap/CRD.
- `nacos`: config + service discovery.

### 3.2 Tool sources

```go
type ToolSource interface {
    ListTools(ctx context.Context) ([]Tool, error)
    Call(ctx context.Context, tool string, args json.RawMessage) (Result, error)
    Health(ctx context.Context) error
}
```

- `nativeMCP`: legacy HTTP MCP via internal router; modern MCP client Phase 2.
- `restBridge`: REST bridge exposed as MCP; normalized through internal router.
- `restAdapter`: Go adapter for complex REST (interface only in MVP).

### 3.3 Normalization

All sources become MCP tools in the internal legacy layer:

- native MCP backend → cluster in `mcp_multicluster`;
- REST bridge → MCP backend in `mcp_multicluster`;
- tool name = `backend__tool`;
- registry sorts deterministically.

The modern facade only knows `LegacyClient`; it does not care about source type.

## 4. Protocol behavior

### Modern `/modern/mcp`

- POST only.
- `server/discover`:
  - `supportedVersions`: `["2026-07-28"]`
  - `capabilities`: tools
  - `serverInfo`: gateway identity
  - `ttlMs`, `cacheScope`
- Every request:
  - `params._meta.io.modelcontextprotocol/protocolVersion` must be `2026-07-28`;
  - `clientCapabilities` must be present;
  - `MCP-Protocol-Version`, `Mcp-Method`, `Mcp-Name` headers;
  - header/body consistency.
- `tools/list`: aggregated, sorted, `ttlMs`/`cacheScope`.
- `tools/call`: `resultType: "complete"`; errors mapped.
- No `initialize`, `Mcp-Session-Id`, GET, DELETE, `ping`.

### Legacy `/mcp`

- `initialize` + `notifications/initialized`.
- `Mcp-Session-Id` optional.
- `tools/list` / `tools/call`.
- Aggregated by Envoy `mcp_router`.
- SSE/GET compatibility is Envoy-level; facade not involved.

## 5. Control plane

MVP:

- YAML config (`api/spec/source-v1alpha1.yaml`).
- `internal/controlplane`:
  - load + validate;
  - content hash version;
  - snapshot model;
  - rollback history (in memory + file).
- `internal/envoy`: generate public/internal listener and cluster config.
- `internal/xds`: minimal ADS/CDS server using `go-control-plane`; it serves cluster resources derived from `SourceSpec`. Listener/route resources remain static in the MVP.
- `internal/restadapter`: REST `SourceSpec` → `legacy.Client` for path/query/JSON-body tools.
- Providers: static, OpenAPI, K8s ConfigMap/McpSource manifest, Nacos Config Service; live informer/service discovery is Phase 2.

## 6. Testing

T0–T8 boundary suite:

- T0: spec fixtures, JSON-RPC codec.
- T1: modern protocol boundary.
- T2: tools aggregation/call.
- T3: legacy client/session.
- T4: dual-era coexistence + mixed sources.
- T5: policy (auth/tenant/allowlist/rate limit).
- T6: control plane version/snapshot/rollback.
- T7: resilience/concurrency/race.
- T8: Envoy config validation/integration (optional).

Completion: `go test ./... -race` green; T0–T7 green; T8 documented.

## 7. Out of scope

- Single endpoint B2.
- Full OAuth/OIDC.
- Redis/distributed limiter.
- Live K8s informer/controller and Nacos service discovery.
- Complex REST adapter (OAuth/HMAC/multipart/long-running).
- Full LDS/RDS xDS resource generation.
- MCP resources/prompts/subscriptions/tasks/MRTR.
- Production hardening of WIP Envoy filters.