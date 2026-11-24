# Progress

## 2026-11-24

- Created repo `E:\Agent_Project\gateway\dual-era-mcp-gateway`.
- `git init -b main`.
- Created `task_plan.md`, `findings.md`, `progress.md`.
- Decision: B1 versioned endpoints + Tool Source/Provider abstractions.
- Scope: static + OpenAPI providers; native MCP + simple REST sources; Nacos/K8s and complex REST deferred.
- Wrote design/spec docs and `api/spec/source-v1alpha1.yaml`.
- Implemented `internal/mcp`, `registry`, `legacy`, `aggregator`, `facade`, `policy`, `legacyendpoint`, `edge`.
- Implemented `internal/source` static + OpenAPI providers, `internal/controlplane` version/rollback/ACK model, `internal/envoy` draft config generation.
- Added `cmd/gateway`, `cmd/legacy-mock`, `cmd/controlplane`.
- Added T0–T7 unit/boundary tests; T4 dual-era test covers mixed native MCP sources.
- Updated PRD + spike with B1/tool-source amendment.
- Added `K8sManifestProvider`, `NacosProvider`, and `internal/restadapter`.
- Added `internal/xds` minimal ADS/CDS server using `go-control-plane`, plus an ADS streaming test.
- Wired xDS apply/rollback into `cmd/controlplane`.
- Rewrote `task_plan.md` as v2: architecture, component responsibilities, ports, WBS by workstream, gates, test plan, risks, runbook, and explicit `[x]/[~]/[ ]/[!]` status.
- Added `docs/spec/traceability.md`: PRD FR-CP/DP/POL/TR/STD/REST/OBS/SEC/OPS/TS + NFR + AC-01..30 mapping.
- Diagnosed GitHub Actions `test` failure: CI runs `-race` (local Windows has no CGO/gcc). Fixed test-only shared writes in facade/legacy/source/REST-adapter tests and the facade's in-place tools slice mutation.
- CI workflow now uses `go-version-file: go.mod`, runs non-race then race, and supports manual dispatch.
- Added `internal/extauthz` and `internal/ratelimit` gRPC services + tests; `cmd/policy`; `cmd/gateway -policy`.
- Added `internal/obs` metrics/trace/JSON logger + tests; `/metrics` and `X-Trace-Id` in local gateway.
- Extended xDS to public CDS/RDS/LDS and ACK/NACK callbacks + tests.
- Added T8 Docker/Envoy harness (`deploy/t8`, `scripts/t8.ps1`) and Helm skeleton.
- Added REST adapter SSRF host checks/allowlist; facade microbenchmarks and `docs/benchmarks/2026-11-25.md`.
- Added `internal/modernclient` (2026-07-28) and `cmd/gateway -modern`; trace propagation to legacy upstream.
- Added ADR-001..007, runbook, demo, k6 script, Helm/T8 docs.
- T8 real Docker/Envoy run PASS (Envoy 1.40.2): fixed Alpine runtime image, PowerShell JSON body, startup retry, teardown exit code; report in `docs/t8-report.md`.
- T8 now covers TLS 1.3 + ALPN h2 + actual HTTP/2, Redis-backed ratelimit (60×200/10×429, DBSIZE=1), and records legacy SSE GET 405.
- T8 image uses `Dockerfile.prebuilt` because Docker-build-time Go module download is blocked.
- Added T8 load run (`scripts/t8-load.ps1`): 3,114 RPS, p95 4.80ms, 0 errors over 5s/10 workers (single-host Docker evidence).
- Added T8 chaos run (`scripts/t8-chaos.ps1`): stopping crm keeps partial catalog and returns 500 for the dead tool; recovery works; stopping policy makes Envoy fail closed with 403; stopping Redis makes ratelimit fall back and still returns 200.
- Added T8 xDS churn (`scripts/t8-xds.ps1`): 100 CDS/RDS applies under live load, 0 errors, 250 ACKs; fixed missing HCM `RouteConfigName`.
- Added T8 mTLS (`scripts/t8-mtls.ps1`): client cert 200, no cert rejected.
- Ran 10-minute T8 load: 500,710 requests, 835 RPS, p99 3.74ms, 0 errors (single host, `tools/list`).
- Added `cmd/crd-controller` + `internal/kube`: McpSource/McpConsumer list/watch, config compile, hash dedupe, Lease leader election, fake API tests. Real-cluster validation still missing.
- T8 now verifies Envoy `/stats/prometheus` downstream metrics in both HTTP and TLS runs.

## Test log

| Date | Command | Result |
|---|---|---|
| 2026-11-24 | git init | OK |
| 2026-11-24 | `go test ./...` | PASS (all packages) |
| 2026-11-24 | `go vet ./...` | PASS |
| 2026-11-24 | `go test -race ./...` | SKIP: `-race requires cgo`; no gcc/clang |
| 2026-11-24 | `scripts/test.ps1` | PASS; Envoy validate SKIP |
| 2026-11-24 | `go test ./... -cover` | PASS; see docs/test-report.md |

## Known gaps

- Envoy runtime T8 not executed: Docker daemon unavailable. Harness is in `deploy/t8`.
- xDS serves public CDS/RDS/LDS; internal `mcp_router` listener is still static.
- TLS/mTLS, SSE, drain, Redis, CRD/controller, k6/chaos, demo/ADR are still open.
- K8s/Nacos providers are manifest/Config-Service based; live informer/service discovery is Phase 2.
- Complex REST (OAuth/HMAC/multipart/long-running) still Phase 2.
- Local race detector unavailable; CI `-race` is green.