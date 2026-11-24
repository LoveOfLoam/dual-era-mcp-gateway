# T8 Envoy runtime harness

This is the real-topology smoke test for B1. It passed on 2026-11-25 with
`envoyproxy/envoy:v1.40.2`; see [`docs/t8-report.md`](../../docs/t8-report.md).

```
client
  └─ public Envoy :8080
       ├─ /mcp ─────────▶ internal Envoy :10001 (mcp_router + mcp_multicluster)
       │                    ├─ weather:9001 (legacy MCP)
       │                    └─ crm:9002     (legacy MCP)
       └─ /modern/mcp ──▶ gateway:8080 (Go modern facade)
                            └─ direct legacy mocks (for this smoke test)
```

## Run

```powershell
cd E:\Agent_Project\gateway\dual-era-mcp-gateway
powershell -File scripts/t8.ps1
```

`scripts/t8.ps1`:

- SKIPs if Docker/Compose is unavailable;
- builds the gateway + legacy-mock image;
- starts public/internal Envoy and two legacy MCP mocks;
- checks `/mcp` initialize + `tools/list`;
- checks `/modern/mcp` `server/discover` + `tools/list`;
- exercises public-Envoy `ext_authz` with the demo key `test`;
- tears the stack down.

## Important limitations

- The default `ENVOY_IMAGE` is `envoyproxy/envoy:v1.40.2`, the baseline used by
  Gate 1. MCP filters are still experimental. For a fully reproducible run, set
  the digest from `docs/spike-envoy-mcp.md`:

  ```powershell
  $env:ENVOY_IMAGE="envoyproxy/envoy@sha256:..."
  ```

- This harness includes `ext_authz` and the Envoy `ratelimit` filter (70 calls
  produce 60×200 / 10×429 with the demo RPM=60 policy). It does **not** include
  Redis-backed limits, REST bridge, SSE long connections, TLS runtime, or
  100-change zero-downtime. Those are separate gates.
- The **TLS overlay** (`docker-compose.tls.yml` + `envoy-public-tls.yaml` +
  `scripts/gen-dev-cert.ps1`) is exercised: `scripts/t8.ps1 -TLS` PASSes the
  same initialize/list/ratelimit checks over HTTPS, with TLSv1.3 + ALPN h2.
- Public routes/clusters include conservative retry (`reset,connect-failure`),
  circuit-breaker and outlier-detection settings. They are not fault-injected
  or validated here; retrying mutating `tools/call` on 5xx is deliberately not
  enabled.
## Optional TLS overlay

```powershell
powershell -File scripts/t8.ps1 -TLS
```

Then send the same authenticated `server/discover` or `tools/list` requests as
the T8 script, but to `https://127.0.0.1:8443` with `curl --http2 -k`.
`/healthz` is behind ext_authz in the current config; use Envoy admin `:9901/ready`
for readiness. The cert is 2 days and for local dev only.

- `mcp_router` and `mcp_multicluster` are WIP Envoy filters. If the image does
  not contain them, the harness must fail rather than silently use a fallback.
- The modern facade points directly at the two mocks in this harness. A later
  topology should point it at the internal Envoy/router and account for the
  router's prefix behavior.