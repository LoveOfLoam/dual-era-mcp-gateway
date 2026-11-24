# T8 runtime report — 2026-11-25

**Result: PASS** on Docker Desktop (Windows) with
`envoyproxy/envoy:v1.40.2`.

This is the real public→internal Envoy topology, not a mocked Go test.

## Topology

```
client :8080
  -> public Envoy
       /mcp          -> internal Envoy :10001 (mcp_router + mcp_multicluster)
       /modern/mcp   -> Go gateway :8080 (modern facade)
  internal Envoy -> weather:9001, crm:9002 (legacy MCP mocks)
  public Envoy  -> policy:19001 (ext_authz)
```

## Command

```powershell
docker compose -f deploy/t8/docker-compose.yml down -v
powershell -File scripts/t8.ps1
```

Observed output (abridged):

```text
==> docker compose up --build
==> legacy /mcp initialize + tools/list
==> modern /modern/mcp discover + tools/list
T8 PASS: public path split + internal mcp_router + modern facade
t8_exit=0
```

## Checks covered

- public Envoy path split: `/mcp` vs `/modern/mcp`;
- internal `mcp_router` + `mcp_multicluster`;
- legacy `initialize` returns `Mcp-Session-Id`;
- legacy `tools/list` contains `weather__get_weather` and `crm__get_customer`;
- modern `server/discover` returns 200;
- modern `tools/list` contains both namespaced tools;
- public Envoy `ext_authz` allows the demo key `test`.

## Issues fixed during the run

1. **gcr.io distroless unreachable** → runtime base image changed to
   `alpine:3.20` (static binaries, non-root user).
2. **Windows PowerShell JSON body mangling** → `scripts/t8.ps1` now uses
   `curl.exe` with a UTF-8 no-BOM body file. Without this, Envoy's `mcp`
   filter logged `mcp_not_jsonrpc` and `mcp_router` returned
   `Invalid or missing MCP request`.
3. **Startup race** → the script retries legacy initialize and modern discover
   for up to 30s while policy/backends warm up.
4. **Teardown stderr treated as failure** → teardown runs with
   `$ErrorActionPreference = "Continue"`, so a successful run exits 0.

## Not covered by this run

- TLS/mTLS (TLS overlay exists but is not exercised);
- ratelimit filter wiring / Redis;
- REST bridge on the internal listener;
- SSE long connections and zero-downtime LDS/CDS churn;
- multi-instance HA, canary, K8s.

`mcp_router`/`mcp_multicluster` remain WIP Envoy filters; the image is pinned
to `v1.40.2` for reproducibility. Use a digest for production.