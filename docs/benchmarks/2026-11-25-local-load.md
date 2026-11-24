# Local Go-mode load run (2026-11-25)

This is **local Go-mode evidence**, not an Envoy/T8 benchmark.

## Topology

```
loadgen (10 workers, 3s)
  -> cmd/gateway :8281 (modern facade)
       -> cmd/legacy-mock :9201 (weather)
```

No Envoy, no TLS, no policy, no SSE, no Docker.

## Command

```powershell
powershell -File scripts/load-local.ps1 -Concurrency 10 -Seconds 3
```

## Result (`docs/benchmarks/local-load.json`)

| Metric | Value |
|---|---:|
| Requests | 23,467 |
| Errors | 0 |
| HTTP 200 | 23,458 |
| RPS | ~7,822 |
| p50 | 1.09 ms |
| p95 | 2.16 ms |
| p99 | 2.71 ms |
| max | 27.7 ms |

Durations in the JSON are nanoseconds; the table converts to milliseconds.

## What this proves

- the modern facade + legacy client + mock path handles this local concurrency
  without errors or obvious queue collapse;
- `cmd/loadgen` and `scripts/load-local.ps1` are reproducible.

## What this does not prove

- Envoy `mcp_router`/`mcp_multicluster` performance;
- TLS/HTTP2, SSE long connections, multi-instance, Redis, xDS churn;
- the PRD target of 500 RPS for 10 minutes with p99 < 10 ms under the real
  topology. A local 3-second run is not that test.

Do not copy this table into a resume as "production 500 RPS" evidence.