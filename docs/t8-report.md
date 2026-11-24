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
# TLS overlay:
powershell -File scripts/t8.ps1 -TLS
```

Observed output (abridged):

```text
==> docker compose up --build
==> legacy /mcp initialize + tools/list
==> legacy tools/call + ratelimit 429
==> ratelimit observed: 200=60 429=10 redis_keys=1
==> modern /modern/mcp discover + tools/list
T8 PASS (HTTP): public path split + internal mcp_router + modern facade + ratelimit
t8_exit=0

# TLS run:
T8 PASS (TLS): public path split + internal mcp_router + modern facade + ratelimit
tls_exit=0
```

The TLS run also executes `cmd/h2check`, a Go `http2.Transport` client:

```text
==> actual HTTP/2: status=200 proto=HTTP/2.0
```

TLS also negotiated `TLSv1.3` + `ALPN protocol: h2` (checked with
`openssl s_client`). Windows `curl.exe` lacks `--http2`, so the Go client is
used for the actual HTTP/2 request.

## Checks covered

- public Envoy path split: `/mcp` vs `/modern/mcp`;
- internal `mcp_router` + `mcp_multicluster`;
- legacy `initialize` returns `Mcp-Session-Id`;
- legacy `tools/list` contains `weather__get_weather`, `crm__get_customer`, and
  `rest__get_customer` (REST bridge on an internal Envoy listener);
- `rest__get_customer` call reaches the REST mock and returns `Ada Lovelace`;
- modern `server/discover` returns 200;
- modern `tools/list` contains both namespaced tools;
- public Envoy `ext_authz` allows the demo key `test`;
- Envoy admin `/stats/prometheus` contains `envoy_http_downstream_rq_total`;
- `mcp_json_rest_bridge` on its own internal listener, aggregated by
  `mcp_router` as `rest__*`;
- Envoy `ratelimit` filter + Go ratelimit service + Redis Lua: 60 calls allowed,
  10 calls returned 429 in the 70-call burst (policy RPM=60); `redis-cli DBSIZE`
  showed the limiter key.
- TLS overlay: same initialize/list/ratelimit checks over HTTPS; TLSv1.3 + ALPN h2
  negotiated; actual HTTP/2 `initialize` returns `status=200 proto=HTTP/2.0`;
- legacy SSE through `cmd/sse-shim`: endpoint event + initialize + tools/list.

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
5. **Ratelimit descriptors were empty** → ext_authz removed `x-tenant-id` and
   then added it, so the ratelimit filter saw no tenant. Removed `x-tenant-id`
   from `HeadersToRemove`; 70 calls then produced 60×200 / 10×429.
6. **Catalog fanout race** → `tools/list` could return only one backend right
   after startup; the script now retries until both tools appear.

## SSE transport

Raw `mcp_router` GET `/mcp` returns 405, so the public listener routes
`GET /mcp` and `/mcp/message` to `cmd/sse-shim`; POST `/mcp` stays on
`mcp_router`. The shim implements the legacy SSE endpoint event and forwards
JSON-RPC messages to the internal Envoy.

T8 runs `cmd/ssecheck`:

```text
==> legacy SSE transport
==> sse_ok=true tools=5
```

This is a real SSE transport check, not a mock.

## Not covered by this run

- `mcp_router`'s raw GET/SSE (405); the shim path is covered instead.
- SSE long connections and zero-downtime LDS/CDS churn;
- multi-instance HA, canary, K8s.

`mcp_router`/`mcp_multicluster` remain WIP Envoy filters; the image is pinned
to `v1.40.2` for reproducibility. Use a digest for production.

Client-certificate enforcement is in [`t8-mtls.md`](t8-mtls.md); xDS churn is
in [`t8-xds.md`](t8-xds.md).