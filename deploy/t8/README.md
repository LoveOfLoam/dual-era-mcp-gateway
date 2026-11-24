# T8 Envoy runtime harness

This is the real-topology smoke test for B1:

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

- The default `ENVOY_IMAGE` is `envoyproxy/envoy:dev` because MCP filters are
  experimental. For a reproducible run, set the digest from `docs/spike-envoy-mcp.md`:

  ```powershell
  $env:ENVOY_IMAGE="envoyproxy/envoy@sha256:..."
  ```

- This harness includes a minimal `ext_authz` check, but does **not** include
  TLS, ratelimit wiring, Redis, REST bridge, SSE long connections, or
  100-change zero-downtime. Those are separate gates; do not claim them from
  this smoke test.
- `mcp_router` and `mcp_multicluster` are WIP Envoy filters. If the image does
  not contain them, the harness must fail rather than silently use a fallback.
- The modern facade points directly at the two mocks in this harness. A later
  topology should point it at the internal Envoy/router and account for the
  router's prefix behavior.