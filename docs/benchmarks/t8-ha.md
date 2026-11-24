# T8 gateway replica-stop run — 2026-11-25

**Result: PARTIAL** — stopping one of two gateway replicas under load produced
3 transient 503s out of 17,707 requests; this is not zero-loss HA.

## Setup

- two explicit gateway services (`gateway`, `gateway-b`) behind the Envoy
  `modern_gateway` cluster;
- Envoy `STRICT_DNS` + active `/healthz` check (1s);
- gateway `/healthz` returns 503 while draining, then waits 8s before shutdown;
- loadgen: 10 workers, 6s, modern `tools/list`, Bearer auth.

## Result

```text
requests=17707 errors=3 rps=2951.1
p50=3.20ms p95=5.30ms p99=6.85ms
status: 200=17694, 503=3
samples:
  503 upstream connect error ... connection termination
```

Explicit endpoints did not remove the stop-boundary 503s. Increasing the drain
wait from 6s to 8s did not change the count.

## Conclusion

- drain + active health check did not eliminate the stop-boundary 503s;
- with two explicit endpoints the error count was still 3/17,707;
- not claimed as HA. Needs Envoy connection draining / EDS or a service-mesh
  endpoint model, not just DNS + health checks.

`scripts/t8-ha.ps1` intentionally exits non-zero when errors > 0.