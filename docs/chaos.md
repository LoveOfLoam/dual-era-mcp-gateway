# Chaos / failure-injection matrix

Actual T8 results are in [`chaos-report.md`](chaos-report.md): partial backend
failure (200 partial catalog), recovery, and ext_authz fail-closed (403).

This is a **test plan, not an executed report**. Docker/Envoy are unavailable in
this environment, so none of the runtime rows below are claimed as passed.

## Preconditions

- T8 stack is running (`deploy/t8/docker-compose.yml`);
- public Envoy reachable on `:8080` (or `:8443` with the TLS overlay);
- demo policy key `test` loaded;
- metrics scraped from gateway `/metrics` and both Envoy admin ports
  `/stats/prometheus`.

## Matrix

| # | Fault | Injection | Expected | Must not |
|---|---|---|---|---|
| C1 | gateway process dies | `docker compose kill gateway` | public `/modern/mcp` returns 502/503; `/mcp` legacy still works | hang or return 200 with empty body |
| C2 | one MCP backend dies | `docker compose stop weather` | `/modern/mcp` tools/list keeps `crm__*`; call to dead tool returns JSON-RPC error | entire catalog disappear |
| C3 | backend 5xx | mock returns 500 | adapter/facade returns JSON-RPC `-32603`; 5xx counter rises | retry a mutating `tools/call` |
| C4 | latency | add 2s delay in mock | p95 rises; no unbounded queue; timeout errors are explicit | goroutine/file-descriptor leak |
| C5 | Redis down | stop Redis with `cmd/policy -redis-addr` | ratelimit falls back to memory (fail-open), auth still fail-closed | auth bypass |
| C6 | policy service down | stop `policy` | public Envoy denies with 5xx/503 (ext_authz fail-closed) | requests pass unauthenticated |
| C7 | xDS NACK | apply an invalid Envoy resource | ACK store records NACK; previous config remains | control plane reports success |
| C8 | bad config | `POST /api/v1/apply` with duplicate prefix | 400, no snapshot/version change | partial apply |
| C9 | SSE disconnect | close client mid-stream | Envoy/backend cleans up; no stuck session | process crash |
| C10 | body limit | send >1 MiB | 413 at Envoy or JSON-RPC invalid request | OOM |

## Commands

```powershell
# terminal 1
docker compose -f deploy/t8/docker-compose.yml up --build

# terminal 2
docker compose -f deploy/t8/docker-compose.yml stop weather
curl.exe -sS -X POST http://127.0.0.1:8080/modern/mcp `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer test" `
  -H "MCP-Protocol-Version: 2026-07-28" `
  -H "Mcp-Method: tools/list" `
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}'
```

## Evidence to record

For each row:

- commit SHA and Envoy image digest;
- command and timestamp;
- response status/body;
- relevant `/metrics` and Envoy stats before/after;
- logs (gateway/Envoy/policy);
- pass/fail and a link to the artifact.

Do not label a row "passed" without the response and metrics evidence.