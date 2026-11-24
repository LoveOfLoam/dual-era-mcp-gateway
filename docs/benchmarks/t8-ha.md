# T8 gateway replica-stop run — 2026-11-25

**Result: PARTIAL** — stopping one of two gateway replicas under load produced
2 transient 503s out of 17,274 requests; this is not zero-loss HA.

## Setup

- `docker compose up --scale gateway=2`
- Envoy `modern_gateway` cluster: `STRICT_DNS` + active `/healthz` check (1s)
- gateway `/healthz` returns 503 while draining, then waits 6s before shutdown
- loadgen: 10 workers, 6s, modern `tools/list`, Bearer auth

## Result

```text
requests=17274 errors=2 rps=2878.9
p50=3.23ms p95=5.20ms p99=6.42ms
status: 200=17262, 503=2
samples:
  503 upstream connect error ... remote connection failure
  503 upstream connect error ... connection termination
```

## Conclusion

- Graceful `/healthz` drain reduced errors from 3 to 2 but did not eliminate them;
- the two 503s happen at the stop boundary;
- the T8 Envoy `STRICT_DNS` cluster appears to keep a single resolved endpoint in
  this setup, so retries hit the stopped replica;
- not claimed as HA. Needs EDS/multi-endpoint xDS or a service-mesh-style
  endpoint set, plus connection draining at the data plane.

`scripts/t8-ha.ps1` intentionally exits non-zero when errors > 0.