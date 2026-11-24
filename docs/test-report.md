# Test Report — 2026-11-24

## Environment

| 项 | 值 |
|---|---|
| OS | Windows 11 |
| Go | go1.26.4 windows/amd64 |
| C compiler | not installed (gcc/clang absent) |
| Envoy | not installed |
| Docker | daemon not reachable from the sandbox |
| Git | 2.48.1 |

## Commands

```powershell
go build ./...
go test ./...
go vet ./...
powershell -File scripts/test.ps1
```

## Results

| Suite | Result |
|---|---|
| `internal/mcp` | PASS |
| `internal/registry` | PASS |
| `internal/legacy` | PASS |
| `internal/facade` | PASS |
| `internal/policy` | PASS |
| `internal/aggregator` | PASS |
| `internal/legacyendpoint` | PASS |
| `internal/edge` | PASS |
| `internal/source` | PASS |
| `internal/controlplane` | PASS |
| `internal/envoy` | PASS |
| `test/boundary` | PASS |
| `go vet ./...` | PASS |
| `scripts/test.ps1` | PASS (Envoy validate SKIP) |

## Coverage (selected)

| Package | Coverage |
|---|---:|
| `internal/controlplane` | 90.4% |
| `internal/edge` | 100.0% |
| `internal/envoy` | 94.4% |
| `internal/policy` | 89.0% |
| `internal/registry` | 88.0% |
| `internal/legacy` | 81.3% |
| `internal/facade` | 80.0% |
| `internal/aggregator` | 73.2% |
| `internal/mcp` | 71.2% |
| `internal/source` | 70.4% |

## Boundary mapping

| Stage | Covered by |
|---|---|
| T0 | `internal/mcp` tests |
| T1 | `internal/facade` tests |
| T2 | `internal/facade` tools tests, `internal/registry` |
| T3 | `internal/legacy`, `internal/legacyendpoint` |
| T4 | `test/boundary/dual_era_test.go` |
| T5 | `internal/policy`, `internal/facade/policy_test.go` |
| T6 | `internal/controlplane`, `internal/envoy` |
| T7 | `internal/facade/resilience_test.go` |
| T8 | **SKIP** — no Envoy binary; `scripts/envoy-validate.ps1` reports SKIP |

## Skipped / limitations

1. **`go test -race`** requires CGO and a C compiler. This Windows host has neither.
   - CI should run `CGO_ENABLED=1 go test -race ./...` on Linux.
2. **Envoy runtime T8** requires an Envoy binary or Docker.
   - `deploy/envoy/bootstrap.yaml` is available for `envoy --mode validate`.
   - Generated full topology is served by `cmd/controlplane` at `/api/v1/envoy`.
3. **Docker integration** is not executed because the sandbox cannot reach the Docker daemon.
4. **xDS server** is not implemented in this MVP; the control plane currently provides versioned config, rollback, ACK model, and generated Envoy config.
5. **K8s/Nacos providers** and complex REST adapter are interfaces/design only.

## Verdict

T0–T7 pass under `go test ./...` and `go vet ./...`. T8 is documented and runnable where Envoy is available. The remaining environment gaps are explicitly recorded rather than hidden.