# Test Report — 2026-11-25

## Environment

| 项 | 值 |
|---|---|
| OS | Windows 11 |
| Go | go1.26.4 windows/amd64 |
| C compiler | not installed (gcc/clang absent) |
| Envoy | not installed as a binary; T8 runs via Docker |
| Docker | Docker Desktop 29.7.2; named-pipe access requires full sandbox permission |
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
| `internal/stdio` | PASS (helper process) |
| `internal/admission` | PASS |
| `internal/controller` | PASS |
| `internal/load` | PASS |
| `internal/audit` | PASS |
| `internal/obs` | PASS |
| `internal/xds` | PASS (ADS CDS/RDS/LDS + ACK/NACK) |
| `test/boundary` | PASS |
| `go vet ./...` | PASS |
| `scripts/test.ps1` | PASS (Envoy validate SKIP) |
| `scripts/t8-load.ps1` | PASS: 3,114 RPS, p95 4.80ms, 0 errors (5s) |
| `scripts/t8-chaos.ps1` | PASS: partial backend 200, dead call 500, recovery, policy-down 403, Redis-down 200 fallback |

## Coverage (selected)

| Package | Coverage |
|---|---:|
| `internal/controlplane` | 90.4% |
| `internal/audit` | 90.9% |
| `internal/envoy` | 94.4% |
| `internal/extauthz` | 93.5% |
| `internal/ratelimit` | 82.4% (memory + Redis Lua/miniredis) |
| `internal/registry` | 88.0% |
| `internal/restadapter` | 87.7% |
| `internal/edge` | 83.3% |
| `internal/policy` | 69.2% (JWT/JWKS added) |
| `internal/legacy` | 82.5% |
| `internal/xds` | 81.9% |
| `internal/facade` | 80.8% |
| `internal/modernclient` | 77.6% |
| `internal/stdio` | 70.7% |
| `internal/admission` | 79.4% |
| `internal/controller` | 68.0% |
| `internal/load` | 89.9% |
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
| T8 | **PASS** — HTTP + TLS (`-TLS`) with `envoyproxy/envoy:v1.40.2`; ext_authz + Redis ratelimit (60×200 / 10×429) + REST bridge (`rest__get_customer`); TLS1.3+ALPN h2 + actual HTTP/2 200; see `docs/t8-report.md` |

## Skipped / limitations

1. **`go test -race`** requires CGO and a C compiler. This Windows host has neither.
   - CI should run `CGO_ENABLED=1 go test -race ./...` on Linux.
2. **Envoy binary validation** is not run locally (`envoy` binary absent); T8 uses the Docker image instead.
   - `deploy/envoy/bootstrap.yaml` is available for `envoy --mode validate` elsewhere.
3. **T8 is executed** with Docker Desktop + Envoy 1.40.2; result is PASS for path split,
   internal `mcp_router`, and modern facade. TLS/SSE/ratelimit/Redis are not part of
   that run; see `docs/t8-report.md`.
4. **xDS** now serves public CDS/RDS/LDS plus ACK/NACK callbacks. The internal `mcp_router` listener is still static; TLS/SSE/zero-downtime are not validated.
5. **K8s/Nacos** are implemented as manifest/Config-Service providers, not live informer/service-discovery clients. Helm is a skeleton; no cluster/CRD validation. Complex REST multipart/binary/long-running remains Phase 2; OAuth2 client credentials and HMAC are implemented.
6. **Dependency/image scanning** is available via `scripts/scan.ps1`, but govulncheck/trivy are not installed here (SKIP).

## Verdict

T0–T7 pass under `go test ./...` and `go vet ./...`; `internal/xds` passes ADS CDS/RDS/LDS and ACK/NACK tests. T8 passes against the real Docker/Envoy topology (Envoy 1.40.2) for the base path split, `mcp_router`, ext_authz, Redis-backed ratelimit (60×200 / 10×429, `DBSIZE=1`), REST bridge (`rest__get_customer`), modern facade, and the TLS overlay (TLS1.3 + ALPN h2 + actual HTTP/2 initialize). SSE remains separate (legacy GET returns 405).