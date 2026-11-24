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

- Envoy runtime T8 not executed: no Envoy binary / Docker daemon.
- xDS server deferred; MVP has versioned config, rollback, ACK model, generated config.
- K8s/Nacos providers and complex REST adapter are interfaces/design only.
- Local race detector unavailable; CI should run `CGO_ENABLED=1 go test -race ./...`.