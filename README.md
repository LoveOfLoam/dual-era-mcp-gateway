# dual-era-mcp-gateway

A **dual-era MCP gateway** with versioned endpoints:

| Endpoint | Client era | Transport | Session |
|---|---|---|---|
| `/mcp` | legacy 2025-xx | Envoy `mcp_router` (or Go legacy endpoint in local mode) | `Mcp-Session-Id` |
| `/modern/mcp` | modern 2026-07-28 | Go modern facade | none |

It supports **two tool sources**:

- **native MCP Server** (legacy HTTP, plus a 2026-07-28 modern client in `internal/modernclient`);
- **RESTful API → MCP** (simple JSON REST via Envoy `mcp_json_rest_bridge`, plus a Go `restadapter` for path/query/JSON-body tools).

And **two config layers**:

- **Provider**: static YAML, OpenAPI, Kubernetes ConfigMap/McpSource manifest, and Nacos Config Service.
- **ToolSource**: normalized native MCP / REST behind one `ListTools`/`Call`/`Health` contract.

Policy and operations:

- ext_authz gRPC (`cmd/policy`) for API key → tenant / allowlist.
- ratelimit gRPC with in-process fixed-window limits and optional Redis Lua fixed-window (`-redis-addr`).
- `/metrics` Prometheus text + `X-Trace-Id` propagation.
- T8 Docker/Envoy harness (PASS on Envoy 1.40.2) and Helm chart skeleton.

Planning and evidence:

- [Project status](docs/project-status.md) — authoritative current progress, highlights, boundaries, blockers, production gap.
- [Detailed task plan](task_plan.md) — WBS, gates, risks, runbook, current status.
- [PRD → implementation traceability](docs/spec/traceability.md) — historical FR/AC mapping; status is stale, use project-status.md first.
- [Test report](docs/test-report.md) — commands, coverage, skipped environment items.
- [Blockers](docs/blockers.md) — environment-specific items that are not PASS.

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
go run ./cmd/gateway -addr :8080 `
  -legacy "weather=http://127.0.0.1:9001/mcp,crm=http://127.0.0.1:9002/mcp" `
  -modern "modern=http://127.0.0.1:9003/mcp" `
  -policy config/policy.example.json
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
- `POST /api/v1/apply` — validate/apply a new `controlplane.Config`; optional
  `X-Actor` / `X-Reason` are recorded.
- `POST /api/v1/rollback` — `{"version":"sha256:..."}`.
- `GET /api/v1/audit` — bounded apply/rollback audit log.
- `GET /metrics` — apply/rollback/ACK/NACK counters.
- `GET /api/v1/envoy` — generated draft Envoy topology.
- `:18000` — ADS server (`internal/xds`); it serves CDS clusters, the public RDS route config, and the public LDS listener.

Current xDS scope: **public listener CDS/RDS/LDS**. The internal `mcp_router` listener remains statically generated in the T8/Helm configs.

## Tests

```powershell
go test ./...
go vet ./...
powershell -File scripts/test.ps1
```

Optional checks:

```powershell
powershell -File scripts/bench.ps1          # in-process facade microbenchmarks
powershell -File scripts/helm-lint.ps1     # SKIP without Helm
powershell -File scripts/envoy-validate.ps1
powershell -File scripts/t8.ps1            # Docker/Envoy T8 smoke test
```

Current status (2026-11-25):

- T0–T7 boundary tests: **green**.
- Go race: **green** (`go test -race -count=1 ./...`, Linux Go container).
- CI (Go only): **green** at `49e4c54`; T8 is not in CI.
- `scripts/test.ps1`: green; Helm/Envoy checks **SKIP** without binaries.
- T8 (`envoyproxy/envoy:v1.40.2`, Docker Desktop): **PASS** for path split,
  ext_authz, Redis ratelimit (60×200 / 10×429), REST bridge, SSE shim,
  TLS1.3 + actual HTTP/2, mTLS, 100 LDS/CDS/RDS churns + ACK/NACK + recovery,
  long-lived SSE across churn, and Envoy Prometheus stats.
- 10-minute T8 load: 500,710 requests, ~835 RPS, p99 3.74ms, 0 errors
  (single host; not a production SLA).
- HA / Redis Sentinel / real K8s are **PARTIAL or BLOCKED**; see
  [`docs/project-status.md`](docs/project-status.md) and
  [`docs/blockers.md`](docs/blockers.md).

See [`docs/test-report.md`](docs/test-report.md) and [`docs/spec/acceptance-tests.md`](docs/spec/acceptance-tests.md).

## Project layout

```
cmd/
  gateway/          # local dual-era server (no Envoy)
  controlplane/     # config/version/rollback + generated Envoy config
  policy/           # ext_authz + ratelimit gRPC services
  legacy-mock/      # local legacy MCP server
  loadgen/          # local load generator CLI
  stdio-bridge/     # stdio MCP -> HTTP /mcp bridge
  sse-shim/         # legacy GET/SSE -> internal POST MCP
  ssecheck/         # SSE transport checker
  admission/        # ValidatingAdmissionWebhook
  controller/       # ConfigMap directory reconcile -> control plane
internal/
  mcp/              # JSON-RPC, _meta, headers, errors
  facade/           # 2026-07-28 endpoint
  legacy/           # legacy HTTP client + session
  modernclient/     # 2026-07-28 stateless MCP client
  stdio/            # stdio JSON-RPC bridge + HTTP handler
  sse/              # legacy SSE transport shim
  admission/        # AdmissionReview validation for McpSource/McpConsumer
  controller/       # directory/ConfigMap -> controlplane.Config reconcile
  load/             # local HTTP load generator
  aggregator/       # multi-source normalization
  registry/         # prefix/sort/collision
  policy/           # API key/tenant/allowlist/rate limit
  extauthz/         # Envoy ext_authz gRPC server
  ratelimit/        # Envoy RateLimitService gRPC server
  source/           # SourceSpec + static/OpenAPI/K8s/Nacos providers
  restadapter/      # REST SourceSpec -> legacy MCP client
  controlplane/     # config validation, versions, rollback, ACK model
  envoy/            # draft Envoy config generation
  xds/              # ADS CDS/RDS/LDS + ACK/NACK callbacks
  edge/             # local path dispatch
  legacyendpoint/   # local legacy /mcp
  obs/              # metrics, JSON logger, trace propagation
test/boundary/      # T4 dual-era boundary tests
docs/spec/          # requirements, protocol matrix, tool sources, providers
docs/superpowers/   # design doc
scripts/            # test/bench/helm/envoy/t8 PowerShell scripts
cmd/crd-controller/ # McpSource/McpConsumer list/watch controller (no client-go)
deploy/envoy/       # bootstrap for validation
deploy/t8/          # Docker/Envoy T8 harness
deploy/helm/dual-era # Helm skeleton
deploy/observability # starter Grafana dashboard + Prometheus rules
deploy/crds/        # McpSource/McpConsumer schemas + list/watch controller
deploy/admission/   # webhook Deployment/Service/configuration skeleton
deploy/controller/  # ConfigMap directory controller Deployment
docs/benchmarks/    # microbenchmark reports
docs/adr/           # ADR-001..008
docs/runbook.md     # operations runbook
docs/demo.md        # 5-minute demo
docs/resume.md      # resume bullets + interview Q&A
docs/chaos.md       # failure-injection matrix; results in chaos-report.md
scripts/k6/         # load-test script (not executed here)
scripts/scan.ps1    # govulncheck/trivy when installed
scripts/load-local.ps1 # local Go-mode load run
scripts/t8-load.ps1  # real Envoy T8 load run
scripts/t8-chaos.ps1 # T8 partial backend / ext_authz failure injection
scripts/t8-mtls.ps1  # T8 client-certificate enforcement
scripts/t8-xds.ps1   # T8 100-change LDS/CDS/RDS churn + ACKs
scripts/t8-ha.ps1    # T8 two replicas, stop one (currently partial)
scripts/t8-redis-ha.ps1 # Redis Sentinel promotion experiment (currently blocked)
```

## Scope and non-goals

Implemented:

- versioned dual-era endpoints;
- native MCP + REST tool-source abstraction;
- legacy and 2026-07-28 native MCP clients;
- stdio MCP bridge (`cmd/stdio-bridge`);
- admission webhook handler + manifests (not cluster-validated);
- ConfigMap directory controller;
- CRD list/watch controller + Lease leader election (`cmd/crd-controller`, fake API tested only);
- static/OpenAPI/K8s-manifest/Nacos providers;
- generic REST adapter (path/query/JSON body, OAuth2 client credentials, HMAC signatures);
- tool prefix/sort/collision;
- API key/tenant/allowlist/rate limit (memory + optional Redis Lua);
- HS256/RS256 JWT validation, including JWKS fetch/cache;
- ext_authz + ratelimit gRPC services;
- config validation/version/rollback/ACK model;
- public LDS/RDS plus CDS and ACK/NACK callbacks;
- draft Envoy topology generation;
- `/metrics`, Envoy `/stats/prometheus` (T8-checked), trace-id propagation, JSON logger;
- T8 Docker/Envoy harness and Helm skeleton;
- 2026-07-28 modern MCP client + `cmd/gateway -modern`;
- in-process facade microbenchmarks;
- JSONL audit events, dry-run config diff;
- ADR-001..008, runbook, demo, k6 script, starter Grafana/Prometheus assets;
- boundary tests T0–T7 + ADS streaming tests.

Not implemented / not verified:

- raw `mcp_router` GET/SSE (405); the public SSE path is `cmd/sse-shim` and is T8-tested;
- single-endpoint B2;
- full modern-upstream Envoy routing (the Go client exists, but Envoy/Helm does not route to it yet);
- internal `mcp_router` LDS resources;
- zero-loss gateway HA (T8 stop-one: 17,707 req / 3×503);
- real-cluster validation of the CRD list/watch controller + leader election (fake API only);
- Redis production HA/rotation (Sentinel promotion blocked by Docker Desktop `+tilt`);
- complex REST adapter multipart/long-running (OAuth2/HMAC are implemented);
- multi-hour SSE soak / production HA load / k6;
- full MCP 2026 conformance, OIDC/SSO, canary, multi-region.

## GitHub

Create an empty GitHub repo named **`dual-era-mcp-gateway`**, then:

```powershell
git remote add origin https://github.com/<you>/dual-era-mcp-gateway.git
git push -u origin main
```

Do not initialize the GitHub repo with a README; this repo already has one.