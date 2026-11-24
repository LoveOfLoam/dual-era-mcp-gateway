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
| `internal/restadapter` | PASS |
| `internal/controlplane` | PASS |
| `internal/envoy` | PASS |
| `internal/extauthz` | PASS |
| `internal/ratelimit` | PASS |
| `internal/modernclient` | PASS |
| `internal/audit` | PASS |
| `internal/obs` | PASS |
| `internal/xds` | PASS (ADS CDS/RDS/LDS + ACK/NACK) |
| `test/boundary` | PASS |
| `go vet ./...` | PASS |
| `scripts/test.ps1` | PASS (Envoy validate SKIP) |

## Coverage (selected)

| Package | Coverage |
|---|---:|
| `internal/controlplane` | 90.4% |
| `internal/audit` | 90.9% |
| `internal/envoy` | 94.4% |
| `internal/extauthz` | 93.5% |
| `internal/ratelimit` | 92.9% |
| `internal/registry` | 88.0% |
| `internal/restadapter` | 87.7% |
| `internal/edge` | 83.3% |
| `internal/policy` | 82.8% |
| `internal/legacy` | 82.5% |
| `internal/xds` | 81.9% |
| `internal/facade` | 80.8% |
| `internal/modernclient` | 77.6% |
| `internal/obs` | 77.4% |
| `internal/aggregator` | 73.2% |
| `internal/mcp` | 71.2% |
| `internal/legacyendpoint` | 65.2% |
| `internal/source` | 63.5% |

## Boundary mapping

| Stage | Covered by |
|---|---|
| T0 | `internal/mcp` tests |
| T1 | `internal/facade` tests |
| T2 | `internal/facade` tools tests, `internal/registry` |
| T3 | `internal/legacy`, `internal/legacyendpoint` |
| T4 | `test/boundary/dual_era_test.go` |
| T5 | `internal/policy`, `internal/facade/policy_test.go` |
| T6 | `internal/controlplane`, `internal/extauthz`, `internal/ratelimit`, `internal/xds` ADS tests |
| T7 | `internal/facade/resilience_test.go` |
| T8 | **SKIP** — no Envoy binary; `scripts/envoy-validate.ps1` reports SKIP |

## Skipped / limitations

1. **`go test -race`** requires CGO and a C compiler. This Windows host has neither.
   - CI should run `CGO_ENABLED=1 go test -race ./...` on Linux.
2. **Envoy runtime T8** requires an Envoy binary or Docker.
   - `deploy/envoy/bootstrap.yaml` is available for `envoy --mode validate`.
   - Generated full topology is served by `cmd/controlplane` at `/api/v1/envoy`.
3. **Docker integration** is not executed because the sandbox cannot reach the Docker daemon. `deploy/t8/docker-compose.yml` and `scripts/t8.ps1` are ready; the script correctly reports SKIP here.
4. **xDS** now serves public CDS/RDS/LDS plus ACK/NACK callbacks. The internal `mcp_router` listener is still static; TLS/SSE/zero-downtime are not validated.
5. **K8s/Nacos** are implemented as manifest/Config-Service providers, not live informer/service-discovery clients. Helm is a skeleton; no cluster/CRD validation. Complex REST (OAuth/HMAC/multipart/long-running) remains Phase 2.

## Verdict

T0–T7 pass under `go test ./...` and `go vet ./...`; `internal/xds` passes ADS CDS/RDS/LDS and ACK/NACK tests. T8 is documented and has a runnable harness, but remains unexecuted here. The remaining environment gaps are explicitly recorded rather than hidden.