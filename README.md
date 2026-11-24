# dual-era-mcp-gateway

A **dual-era MCP gateway** with versioned endpoints:

| Endpoint | Client era | Transport | Session |
|---|---|---|---|
| `/mcp` | legacy 2025-xx | Envoy `mcp_router` (or Go legacy endpoint in local mode) | `Mcp-Session-Id` |
| `/modern/mcp` | modern 2026-07-28 | Go modern facade | none |

It supports **two tool sources**:

- **native MCP Server** (legacy HTTP; modern direct client is Phase 2);
- **RESTful API → MCP** (simple JSON REST via Envoy `mcp_json_rest_bridge`, plus a Go `restadapter` for path/query/JSON-body tools).

And **two config layers**:

- **Provider**: static YAML, OpenAPI, Kubernetes ConfigMap/McpSource manifest, and Nacos Config Service.
- **ToolSource**: normalized native MCP / REST behind one `ListTools`/`Call`/`Health` contract.

## Architecture (B1)

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

- Public Envoy does **not** use terminal `mcp_router`; it only routes by path.
- Internal Envoy owns legacy aggregation; modern facade bridges 2026-07-28 to it.
- The modern facade is stateless; legacy session state stays in the legacy path.

## Quick start (local, no Envoy)

Terminal 1:

```powershell
go run ./cmd/legacy-mock -addr :9001 -name weather -tools get_weather,list_cities
```

Terminal 2:

```powershell
go run ./cmd/legacy-mock -addr :9002 -name crm -tools get_customer
```

Terminal 3:

```powershell
go run ./cmd/gateway -addr :8080 -legacy "weather=http://127.0.0.1:9001/mcp,crm=http://127.0.0.1:9002/mcp"
```

Modern discover:

```powershell
$body = '{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
curl.exe -sS -X POST http://127.0.0.1:8080/modern/mcp `
  -H "Content-Type: application/json" `
  -H "MCP-Protocol-Version: 2026-07-28" `
  -H "Mcp-Method: server/discover" `
  -d $body
```

Modern tools/call:

```powershell
$body = '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"weather__get_weather","arguments":{"q":"Paris"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
curl.exe -sS -X POST http://127.0.0.1:8080/modern/mcp `
  -H "Content-Type: application/json" `
  -H "MCP-Protocol-Version: 2026-07-28" `
  -H "Mcp-Method: tools/call" `
  -H "Mcp-Name: weather__get_weather" `
  -d $body
```

Legacy initialize:

```powershell
curl.exe -sS -D - -X POST http://127.0.0.1:8080/mcp `
  -H "Content-Type: application/json" `
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

## Control plane + xDS

```powershell
go run ./cmd/controlplane -addr :18080 -xds-addr :18000
```

- `GET /api/v1/status` — current config version + xDS version.
- `GET /api/v1/revisions` — history.
- `POST /api/v1/apply` — validate/apply a new `controlplane.Config`.
- `POST /api/v1/rollback` — `{"version":"sha256:..."}`.
- `GET /api/v1/envoy` — generated draft Envoy topology.
- `:18000` — ADS/CDS snapshot server (`internal/xds`); it serves clusters derived from `SourceSpec`.

Current xDS scope: **CDS clusters**; listeners/routes remain statically generated until the resource builder is extended.

## Tests

```powershell
go test ./...
go vet ./...
powershell -File scripts/test.ps1
```

Optional Envoy validation:

```powershell
powershell -File scripts/envoy-validate.ps1
```

Current status (2026-11-24):

- T0–T7 boundary tests: **green**.
- xDS ADS/CDS streaming test: **green**.
- `go vet`: green.
- `scripts/test.ps1`: green; Envoy validation **SKIP** (no `envoy` binary).
- `go test -race`: **SKIP** locally (no C compiler/CGO); run it in Linux CI.

See [`docs/test-report.md`](docs/test-report.md) and [`docs/spec/acceptance-tests.md`](docs/spec/acceptance-tests.md).

## Project layout

```
cmd/
  gateway/          # local dual-era server (no Envoy)
  controlplane/     # config/version/rollback + generated Envoy config
  legacy-mock/      # local legacy MCP server
internal/
  mcp/              # JSON-RPC, _meta, headers, errors
  facade/           # 2026-07-28 endpoint
  legacy/           # legacy HTTP client + session
  aggregator/       # multi-source normalization
  registry/         # prefix/sort/collision
  policy/           # API key/tenant/allowlist/rate limit
  source/           # SourceSpec + static/OpenAPI/K8s/Nacos providers
  restadapter/      # REST SourceSpec -> legacy MCP client
  controlplane/     # config validation, versions, rollback, ACK model
  envoy/            # draft Envoy config generation
  xds/              # minimal ADS/CDS server (go-control-plane)
  edge/             # local path dispatch
  legacyendpoint/   # local legacy /mcp
  obs/              # reserved
test/boundary/      # T4 dual-era boundary tests
docs/spec/          # requirements, protocol matrix, tool sources, providers
docs/superpowers/   # design doc
scripts/            # PowerShell test/demo scripts
deploy/envoy/       # bootstrap for validation
```

## Scope and non-goals

Implemented:

- versioned dual-era endpoints;
- native MCP + REST tool-source abstraction;
- static/OpenAPI/K8s-manifest/Nacos providers;
- generic REST adapter (path/query/JSON body);
- tool prefix/sort/collision;
- API key/tenant/allowlist/rate limit;
- config validation/version/rollback/ACK model;
- draft Envoy topology generation;
- minimal ADS/CDS xDS server;
- boundary tests T0–T7 + ADS streaming test.

Not implemented (Phase 2):

- single-endpoint B2;
- modern MCP upstream;
- full LDS/RDS xDS resources (currently CDS only);
- live Kubernetes informer / Nacos service discovery;
- complex REST adapter (OAuth/HMAC/multipart/long-running);
- real Envoy T8 runtime integration;
- OAuth/OIDC, Redis, canary, multi-region.

## GitHub

Create an empty GitHub repo named **`dual-era-mcp-gateway`**, then:

```powershell
git remote add origin https://github.com/<you>/dual-era-mcp-gateway.git
git push -u origin main
```

Do not initialize the GitHub repo with a README; this repo already has one.