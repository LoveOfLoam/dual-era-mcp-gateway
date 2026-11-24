# Runbook

## Local gateway

```powershell
# mocks
go run ./cmd/legacy-mock -addr :9001 -name weather -tools get_weather,list_cities
go run ./cmd/legacy-mock -addr :9002 -name crm -tools get_customer

# gateway (policy optional)
go run ./cmd/gateway -addr :8080 `
  -legacy "weather=http://127.0.0.1:9001/mcp,crm=http://127.0.0.1:9002/mcp" `
  -policy config/policy.example.json
```

Health: `GET /healthz`, metrics: `GET /metrics`.

## stdio bridge

```powershell
go run ./cmd/stdio-bridge -addr :18081 -command "C:\path\to\mcp-server.exe"
# arguments are comma-separated:
go run ./cmd/stdio-bridge -addr :18081 -command node -args "server.js,--stdio"
```

This is a single child process; no process pool or automatic restart yet.
Expose it to the internal Envoy `mcp_router` as a legacy MCP backend.

## ConfigMap directory controller

```powershell
go run ./cmd/controller -dir ./deploy/controller/example -controlplane http://127.0.0.1:18080 -once
# or poll:
go run ./cmd/controller -dir ./deploy/controller/example -controlplane http://127.0.0.1:18080
```

It applies only when the compiled config hash changes. This is not a CRD
informer; see `deploy/controller/README.md`.

## Admission webhook

```powershell
go run ./cmd/admission -addr :18443
# in a cluster: -cert /tls/tls.crt -key /tls/tls.key
```

The handler validates `McpSource` / `McpConsumer`; `deploy/admission` has
Deployment/Service/ValidatingWebhookConfiguration skeletons with a placeholder
`caBundle`. No cert-manager or cluster test exists.

## Control plane

```powershell
go run ./cmd/controlplane -addr :18080 -xds-addr :18000
```

- `GET /api/v1/status` — config version, xDS version.
- `GET /api/v1/revisions` — history.
- `POST /api/v1/apply` — new `controlplane.Config` JSON.
- `POST /api/v1/rollback` — `{"version":"sha256:..."}`.
- `GET /api/v1/envoy` — draft topology.

## Policy services

```powershell
go run ./cmd/policy -config config/policy.example.json -authz-addr :19001 -ratelimit-addr :19002
# optional distributed limiter:
go run ./cmd/policy -config config/policy.example.json -redis-addr 127.0.0.1:6379
```

The gRPC services are consumed by Envoy in the T8/Helm configs. In local Go
mode, `cmd/gateway -policy` uses the same policy engine in-process.
JWT uses `jwt.hmacSecret` (HS256), `jwt.publicKeyPEM` (RS256), or `jwt.jwksURL`
(RS256 with a 5-minute cache).

## T8

```powershell
powershell -File scripts/t8.ps1
```

- SKIP means Docker/Compose is unavailable; it is not a pass.
- On a real host, pin `ENVOY_IMAGE` to the digest from `docs/spike-envoy-mcp.md`.
- If it fails, inspect `docker compose -f deploy/t8/docker-compose.yml logs`.

## Rollback

1. `GET /api/v1/revisions`.
2. `POST /api/v1/rollback` with the previous `sha256:...`.
3. Check `/api/v1/status`; on a real Envoy, check ACK/NACK.

## Common failures

| Symptom | Check |
|---|---|
| 401 on every request | missing/invalid API key; in T8 check `Authorization: Bearer test` |
| 403 on a tool | consumer allowlist / `Mcp-Name` header |
| 429 | policy RPM; `error.layer` / `retry-after` |
| modern `-32022` | `_meta` protocolVersion is not `2026-07-28` |
| modern `-32020` | header/body method/name mismatch |
| T8 SKIP | Docker daemon/Compose unavailable |
| `go test -race` unavailable | no gcc/CGO; use CI |
| xDS NACK | no live Envoy callback yet; AckStore is model-only |

## CI

CI is `.github/workflows/test.yml`: non-race tests, race tests, vet, build.
It is CI only — there is no CD/deployment workflow.