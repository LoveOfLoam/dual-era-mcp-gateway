# Acceptance Tests (T0–T8)

Completion means T0–T7 green under `go test ./... -race`; T8 runnable/documented where Envoy/Docker exists.

## T0 — Spec and codec

| ID | Test | Pass |
|---|---|---|
| T0-01 | JSON-RPC request decode (id string/number/null) | correct |
| T0-02 | Response encode result vs error | correct |
| T0-03 | `_meta` extraction | correct |
| T0-04 | Header normalization | case-insensitive |
| T0-05 | Error code mapping | -32700/-32600/-32601/-32602/-32020/-32022 |
| T0-06 | Deterministic JSON fixture | golden match |

## T1 — Modern protocol boundary

| ID | Test | Pass |
|---|---|---|
| T1-01 | `server/discover` valid | 200, versions/capabilities/serverInfo |
| T1-02 | Missing `_meta` | 400, -32602 |
| T1-03 | Missing clientCapabilities | 400, -32602 |
| T1-04 | Wrong protocolVersion | 400, -32022 |
| T1-05 | Header/body method mismatch | 400, -32020 |
| T1-06 | Header/body name mismatch | 400, -32020 |
| T1-07 | GET `/modern/mcp` | 405 |
| T1-08 | DELETE `/modern/mcp` | 405 |
| T1-09 | `initialize` on modern | rejected |
| T1-10 | No `Mcp-Session-Id` in response | pass |
| T1-11 | Parse error | 400, -32700 |
| T1-12 | Invalid request | 400, -32600 |
| T1-13 | Unsupported method | -32601 |
| T1-14 | Origin denied | 403 |

## T2 — Tools aggregation/call

| ID | Test | Pass |
|---|---|---|
| T2-01 | `tools/list` two sources | both tools, prefixed |
| T2-02 | Deterministic order | sorted |
| T2-03 | `tools/list` ttlMs/cacheScope | present |
| T2-04 | `tools/call` native MCP | routed, resultType complete |
| T2-05 | `tools/call` REST | routed, resultType complete |
| T2-06 | Unknown tool | -32601 |
| T2-07 | Upstream error | mapped |
| T2-08 | Source failure | healthy source still listed |
| T2-09 | Single source prefix | still prefixed |
| T2-10 | Collision | config rejected |

## T3 — Legacy client/session

| ID | Test | Pass |
|---|---|---|
| T3-01 | `initialize` | 200 + session |
| T3-02 | `notifications/initialized` | 202 |
| T3-03 | `tools/list` | tools |
| T3-04 | `tools/call` | result |
| T3-05 | Session reuse | same session |
| T3-06 | Upstream 404 | re-init once |
| T3-07 | No session | allowed if upstream permits |

## T4 — Dual-era + mixed sources

| ID | Test | Pass |
|---|---|---|
| T4-01 | `/mcp` and `/modern/mcp` simultaneously | both 200 |
| T4-02 | Same tool catalog | names match |
| T4-03 | Native + REST mixed | both callable |
| T4-04 | Modern client → legacy upstream | facade bridges |
| T4-05 | Legacy client → REST source | internal path works |
| T4-06 | Tenant isolation | tenant A cannot see B |
| T4-07 | Health endpoint | reports sources |

## T5 — Policy

| ID | Test | Pass |
|---|---|---|
| T5-01 | No key | 401 |
| T5-02 | Wrong key | 401 |
| T5-03 | Tenant spoof header | ignored |
| T5-04 | Tool not allowed | 403 |
| T5-05 | Tenant rate limit | 429 |
| T5-06 | Tool rate limit | 429, layer=tool-rpm |
| T5-07 | Secrets absent from logs | pass |
| T5-08 | Provider reload | new policy applied |

## T6 — Control plane

| ID | Test | Pass |
|---|---|---|
| T6-01 | Valid config | snapshot built |
| T6-02 | Invalid ref | error |
| T6-03 | Duplicate prefix | error |
| T6-04 | Version hash | deterministic |
| T6-05 | Rollback | old snapshot restored |
| T6-06 | Provider reload | version changes |
| T6-07 | Envoy config generation | non-empty, expected names |
| T6-08 | ACK/NACK model | ack/nack recorded |

## T7 — Resilience/concurrency

| ID | Test | Pass |
|---|---|---|
| T7-01 | Upstream timeout | 502, no panic |
| T7-02 | Partial backend failure | degraded, not 500 |
| T7-03 | 100 concurrent calls | no race, all accounted |
| T7-04 | Graceful shutdown | in-flight completes |
| T7-05 | Large body limit | 413 |
| T7-06 | Header limit | 431 |
| T7-07 | Slow client | timeout, no goroutine leak |

## T8 — Envoy

| ID | Test | Pass |
|---|---|---|
| T8-01 | `envoy --mode validate` | exit 0 |
| T8-02 | Public listener routes `/mcp` | legacy |
| T8-03 | Public listener routes `/modern/mcp` | facade |
| T8-04 | Internal `mcp_router` aggregates | tools |
| T8-05 | REST bridge aggregated | REST tool |
| T8-06 | Header removal | no Authorization leak |
| T8-07 | SSE long connection | no unexpected close |

T8 requires Envoy binary or Docker; if unavailable, record `SKIP` with exact command.

## Commands

```powershell
go test ./... -race
go vet ./...
powershell -File scripts/test.ps1
powershell -File scripts/envoy-validate.ps1
```

## Definition of done

- T0–T7 all green.
- T8 validated or documented skip with environment reason.
- `go test ./... -race` in CI.
- README + test report committed.
- No secrets in repo/logs.