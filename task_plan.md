# Task Plan — dual-era-mcp-gateway

## Goal

Build a dual-era MCP gateway in a new git repository:

- **legacy endpoint** `/mcp`: 2025-xx clients, `initialize` / `Mcp-Session-Id` / SSE-compatible.
- **modern endpoint** `/modern/mcp`: 2026-07-28 clients, `_meta` driven, no sessions, no initialize.
- **Envoy** public listener for L7/TLS/auth/rate-limit/path dispatch.
- **internal Envoy** legacy router for `mcp_router` aggregation.
- **Go modern facade** for 2026-07-28, bridging to legacy upstreams.
- **two tool sources**: native MCP Server and REST-to-MCP.
- **two config provider layers**: static/OpenAPI now; K8s/Nacos later behind interfaces.
- spec-driven + TDD; boundary tests T0–T8.

Completion is defined by `docs/spec/acceptance-tests.md`: all T0–T7 tests green under `go test ./... -race`; T8 Envoy validation/integration runnable where Envoy/Docker is available.

## Non-goals (MVP)

- Single `/mcp` endpoint (B2). We deliberately use versioned endpoints.
- Full OAuth authorization server; only API key / trusted tenant header.
- Redis-backed distributed rate limit; in-memory for MVP.
- K8s CRD controller and Nacos watch; only interfaces + static/OpenAPI providers.
- Full MCP resources/prompts/subscriptions/tasks/MRTR.
- Multi-region, billing, console.
- Production hardening of WIP Envoy MCP filters.

## Architecture

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
  └─ /mcp ─────────▶ Internal Envoy :10001 (legacy clients)
```

- Public Envoy does **not** use terminal `mcp_router`; it only routes by path.
- Internal Envoy owns `mcp_router`; its backends are normalized MCP servers.
- REST tools are normalized through `mcp_json_rest_bridge` (simple) or Go REST adapter (complex).
- Modern facade sees one `LegacyClient`; it does not care whether a tool came from MCP or REST.

## Phases

### Phase 0 — Repo, spec, planning

- [x] Create repo `dual-era-mcp-gateway`.
- [x] `task_plan.md`, `findings.md`, `progress.md`.
- [x] `docs/superpowers/specs/2026-11-24-dual-era-b1-design.md`.
- [x] `docs/spec/requirements.md`.
- [x] `docs/spec/protocol-matrix.md`.
- [x] `docs/spec/tool-sources.md`.
- [x] `docs/spec/providers.md`.
- [x] `docs/spec/acceptance-tests.md`.
- [x] `api/spec/source-v1alpha1.yaml`.

### Phase 1 — T0/T1: protocol + facade tests

- [x] `internal/mcp`: JSON-RPC, `_meta`, headers, errors, discover result.
- [x] `internal/facade`: `server/discover`, `tools/list`, `tools/call`.
- [x] Tests: missing `_meta`, missing capabilities, header mismatch `-32020`, unsupported version `-32022`, no session, GET/DELETE rejection, `resultType`, `ttlMs/cacheScope`.

### Phase 2 — legacy bridge + registry

- [x] `internal/legacy`: initialize, session reuse, list, call, 404 refresh.
- [x] `internal/registry`: prefix `backend__tool`, deterministic sort, collision, partial failure.
- [x] Tests: legacy 2-backend aggregation, single-backend prefix, mixed source.

### Phase 3 — dual-era dispatch + policy

- [x] `internal/policy`: API key, tenant derivation, allowlist, rate limit.
- [x] `internal/edge`: path dispatch `/mcp` vs `/modern/mcp` for tests.
- [x] Tests: legacy and modern simultaneously; 401/403/429; tenant spoof.

### Phase 4 — sources/providers

- [x] `internal/source`: `SourceSpec`, static provider, OpenAPI provider.
- [x] Native MCP + simple REST normalized through `legacy.Client` + `aggregator`.
- [x] `internal/restadapter`: REST SourceSpec -> legacy MCP client (path/query/JSON body).
- [x] Tests: mixed native+REST catalog; source failure; static provider re-Load.
- [x] `K8sManifestProvider` (ConfigMap/McpSource YAML) and `NacosProvider` (Config Service HTTP) behind the same Provider interface.

### Phase 5 — control plane

- [x] `internal/controlplane`: config validation, version, snapshot, rollback.
- [x] `internal/envoy`: generate public/internal listener configs and cluster config.
- [x] Tests: invalid config, version changes, rollback, ACK/NACK model.
- [x] Minimal ADS xDS server (`internal/xds`) using `go-control-plane`; serves CDS clusters and has an ADS streaming test. Listeners/routes are still generated statically by `internal/envoy`.

### Phase 6 — boundary suite + Envoy

- [x] T0–T7 all green under `go test ./...` (local host has no CGO/gcc, so `-race` is CI-only).
- [x] `.github/workflows/test.yml` runs `go test -race ./...` on Linux CI.
- [x] `scripts/test.ps1` supports `-Race`; local run without gcc falls back and warns.
- [x] `scripts/envoy-validate.ps1` runs `envoy --mode validate` when available, SKIP otherwise.
- [x] `deploy/envoy/` bootstrap + generated config endpoint.
- [ ] T8 Docker/Envoy runtime (blocked: Docker daemon inaccessible and Envoy not installed; documented, not faked).

### Phase 7 — docs + commit

- [x] README with quick start, architecture, matrix, test evidence.
- [x] Test report.
- [x] Git history with logical commits.
- [x] GitHub push instructions.

## Decisions

| ID | Decision | Reason |
|---|---|---|
| D-001 | B1 versioned endpoints | Envoy native filters cannot cleanly serve modern+legacy on one listener; versioned endpoints are testable and honest |
| D-002 | Public Envoy routes by path; internal Envoy owns `mcp_router` | Avoid terminal router blocking modern path |
| D-003 | Modern facade is Go | 2026-07-28 aggregation/translation is easier and testable in Go |
| D-004 | Native MCP + REST both normalized into internal MCP backends | One catalog, one policy model |
| D-005 | Providers are interfaces; static/OpenAPI now | Nacos/K8s not block MVP |
| D-006 | TDD boundary tests first | Completion is test-defined |

## Completion criteria

- `go test ./... -race` green.
- T0–T7 in `docs/spec/acceptance-tests.md` green.
- `go vet ./...` green.
- Envoy configs validate where Envoy exists; T8 documented.
- README + test report + git commits.