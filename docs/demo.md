# 5-minute demo

This demo uses the local Go mode (no Envoy). Use `scripts/t8.ps1` on a Docker
host for the real public/internal Envoy topology.

## 0. Start two legacy mocks

```powershell
go run ./cmd/legacy-mock -addr :9001 -name weather -tools get_weather,list_cities
go run ./cmd/legacy-mock -addr :9002 -name crm -tools get_customer
```

## 1. Start the gateway with policy + metrics

```powershell
go run ./cmd/gateway -addr :8080 `
  -legacy "weather=http://127.0.0.1:9001/mcp,crm=http://127.0.0.1:9002/mcp" `
  -policy config/policy.example.json
```

The example policy uses the API key `test` (SHA-256 in
`config/policy.example.json`).

## 2. Modern discovery + list

```powershell
$meta = '"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}'
$body = '{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{' + $meta + '}}'
curl.exe -sS -X POST http://127.0.0.1:8080/modern/mcp `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer test" `
  -H "MCP-Protocol-Version: 2026-07-28" `
  -H "Mcp-Method: server/discover" `
  -d $body
```

`tools/list` is the same request with `"method":"tools/list"` and
`Mcp-Method: tools/list`.

## 3. Show 401 / 403 / 429

- Remove `Authorization` → 401.
- Call `crm__create_order` (not allowed by the example policy) → 403.
- Call `weather__get_weather` twice: the example RPM is 60, so raise the limit or
  use a test policy with `rpm: 1` to show 429 + `error.layer`.

## 4. Metrics

```powershell
curl.exe -sS http://127.0.0.1:8080/metrics
```

Look for `mcp_gateway_requests_total`, `mcp_gateway_auth_denied_total`,
`mcp_gateway_rate_limited_total`.

## 5. Control plane + xDS

```powershell
go run ./cmd/controlplane -addr :18080 -xds-addr :18000
curl.exe -sS http://127.0.0.1:18080/api/v1/status
curl.exe -sS http://127.0.0.1:18080/api/v1/revisions
```

`/api/v1/envoy` returns the draft public/internal topology.

## 6. Honest close

Say:

- T0–T7 and race are green;
- public CDS/RDS/LDS + ext_authz/ratelimit are unit-tested;
- T8 Envoy runtime is not run here (Docker unavailable);
- TLS/SSE/Redis/CRD/controller are still open.

Do not claim production readiness.