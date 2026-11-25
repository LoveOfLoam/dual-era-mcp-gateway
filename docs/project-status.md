# Project status — dual-era MCP gateway

Updated: 2026-11-25  
HEAD: `49e4c54` (CI green)  
Scope: **B1 vertical slice**, not a production-grade gateway

## 1. One-line positioning

> A dual-era MCP gateway vertical slice: Envoy data plane + Go control plane,
> xDS LDS/CDS/RDS delivery, legacy `/mcp` + modern `/modern/mcp` endpoints,
> and native MCP + REST tool sources behind one catalog.

The three deliberate differentiators are:

1. **Control/data-plane separation with xDS**;
2. **Dual legacy/modern endpoints for the protocol transition period**;
3. **Native MCP + REST API → MCP routing** in one gateway.

Everything in this document is either:

- **PASS**: executed and has a reproducible harness/result;
- **PARTIAL**: implemented or partially executed, not enough for a production claim;
- **BLOCKED**: environment/tooling prevents execution;
- **UNVERIFIED**: code/docs exist but no runtime evidence.

## 2. Progress at a glance

| Area | Status | Evidence |
|---|---|---|
| Go unit/boundary tests | PASS | `go test ./...`, `test/boundary` |
| Go race | PASS | Linux Go container `go test -race -count=1 ./...` |
| CI (Go only) | PASS | GitHub Actions #18, `49e4c54` |
| Legacy `/mcp` | PASS | T8 initialize/session/list/call |
| Modern `/modern/mcp` | PASS | T8 discover/list |
| Native MCP backend aggregation | PASS | T8 weather/crm |
| REST bridge | PASS | T8 `rest__get_customer` returns `Ada Lovelace` |
| ext_authz + allowlist | PASS | T8 401/403 cases, policy tests |
| Redis ratelimit | PASS | T8 60×200 / 10×429, `DBSIZE=1` |
| SSE legacy transport | PASS | `cmd/sse-shim`, `sse_ok=true tools=5` |
| TLS 1.3 + ALPN h2 | PASS | T8 `-TLS`, `h2check` |
| mTLS | PASS | client cert 200; no cert rejected |
| xDS LDS/CDS/RDS churn | PASS | 100 applies, 0 request errors, ACK 175 |
| xDS NACK + recovery | PASS | NACK 3143, old listener kept, recovery 200 |
| Long-lived SSE across xDS churn | PASS | 3s hold across 100 applies |
| 10-minute T8 load | PASS (single host) | 500,710 req, ~835 RPS, p99 3.74ms, 0 error |
| T8 chaos | PASS | partial catalog/recovery, auth fail-closed, Redis fail-open |
| Envoy Prometheus stats | PASS | `/stats/prometheus` checked in T8 |
| CRD list/watch controller | PARTIAL | fake Kubernetes API tests only |
| CRD Lease leader election | PARTIAL | fake API acquire/renew/expiry tests |
| Real Kubernetes cluster | BLOCKED | no kubeconfig/kind/helm |
| Zero-loss gateway HA | PARTIAL | 17,707 req / 3×503 |
| Redis Sentinel failover | BLOCKED | Docker Desktop `+tilt` |
| Helm chart validation | BLOCKED | helm not installed |
| Production SLO/HA/security | NOT CLAIMED | see §7 |

## 3. The three highlights, deepened

### 3.1 Control/Data-plane separation and xDS

**Design**

- Public Envoy is configured by the Go control plane through ADS:
  CDS (clusters), RDS (route config), LDS (public listener);
- `cmd/controlplane` keeps config versions, history, rollback, dry-run diff,
  audit events, and ACK/NACK counters;
- internal legacy aggregation (`mcp_router`) stays static in the MVP.

**Evidence**

- T8: 100 LDS/CDS/RDS applies under live load;
- Envoy ACK 175; NACK injection 3143; old listener kept; recovery 200;
- long-lived SSE survived the churn;
- 11,711 requests during the run with 0 transport errors.

**Boundaries**

- only the **public listener** is xDS-managed; internal `mcp_router` is static;
- single control-plane node; no leader election for the control plane;
- snapshots are in-memory; no durable config store;
- no EDS; no multi-node xDS; no control-plane auth/RBAC/mTLS;
- ACK/NACK is recorded, but there is no automated rollback on NACK.

**Next depth work**

- persist snapshots + audit log (file/DB) and reload on restart;
- add control-plane auth/RBAC and lease-based leader election;
- move internal `mcp_router` listener/resource into xDS;
- add EDS or a real endpoint set; add NACK-triggered automatic rollback.

### 3.2 Legacy/modern dual endpoints

**Design**

- `/mcp`: legacy path via Envoy `mcp_router`, with session;
- `/modern/mcp`: Go facade for 2026-07-28 `server/discover`, `_meta`,
  `resultType`, `ttlMs`/`cacheScope`, stateless semantics;
- B1 intentionally keeps two endpoints instead of pretending one endpoint can
  serve both eras.

**Evidence**

- T8 both endpoints pass in the same stack;
- legacy initialize/session/list/call;
- modern discover/list;
- TLS + HTTP/2 and `h2check` actual `HTTP/2.0`;
- SSE shim: public GET `/mcp` + `/mcp/message`, `sse_ok=true tools=5`.

**Boundaries**

- raw `mcp_router` GET/SSE is 405; the public SSE path is a Go shim;
- modern is a Go facade, not an Envoy-native modern filter;
- modern coverage is the core path, not a complete 2026 conformance suite;
- B1 is not the single-endpoint B2 design.

**Next depth work**

- protocol version negotiation matrix and compatibility tests;
- SSE shim session/limits/timeouts and parity with legacy semantics;
- error-code/header edge cases; graceful drain for both endpoints;
- define explicitly which MCP 2026 features are supported.

### 3.3 Native MCP + REST API → MCP

**Design**

- `Provider`/`ToolSource` abstraction;
- native legacy MCP and a 2026-07-28 modern client;
- REST adapter for path/query/JSON body; OAuth2 client credentials and HMAC;
- Envoy `mcp_json_rest_bridge` on an internal listener, aggregated as `rest__*`;
- prefixing, deterministic ordering, collision validation.

**Evidence**

- T8 `rest__get_customer` call returns `Ada Lovelace`;
- `rest__create_order` listed;
- mixed catalog: `weather__*`, `crm__*`, `rest__*`.

**Boundaries**

- `mcp_json_rest_bridge` is an Envoy **WIP** filter;
- multipart/long-running/idempotent REST is not implemented;
- REST auth is client-credentials/HMAC, not full OAuth/OIDC;
- no per-source circuit breaker or full retry budget;
- SSRF protection is metadata host + allowHosts, not a complete egress policy.

**Next depth work**

- retry/timeout/circuit per source; idempotency keys;
- multipart/async REST and webhook callbacks;
- health/readiness per source and partial catalog policy;
- source-level metrics and catalog cache invalidation.

## 4. What T8 actually proved

Docker Desktop + Envoy 1.40.2, not CI and not a production cluster:

- public path split and internal `mcp_router`/`mcp_multicluster`;
- legacy + modern endpoints;
- native + REST mixed catalog;
- ext_authz and Redis ratelimit (60×200 / 10×429);
- TLS 1.3 + ALPN h2 + actual HTTP/2;
- mTLS client certificate enforcement;
- xDS 100 LDS/CDS/RDS applies, ACK/NACK, recovery;
- long-lived SSE across churn;
- 10-minute load: 500,710 requests, ~835 RPS, p99 3.74ms, 0 errors;
- chaos: backend partial failure/recovery, policy fail-closed, Redis fail-open;
- Envoy Prometheus metrics.

Negative results are part of the evidence:

- gateway replica stop: 17,707 requests / 3×503 — not zero-loss HA;
- Redis Sentinel: `+tilt`, no promotion within 15s/40s;
- real K8s: no kubeconfig/kind/helm;
- raw `mcp_router` GET/SSE: 405 (shim required).

## 5. Blockers

See [`blockers.md`](blockers.md) for commands and exact conditions.

- **Kubernetes real cluster**: no context, no kind, no helm;
- **Zero-loss HA**: requires Envoy data-plane draining/EDS; current 3×503;
- **Redis HA**: Sentinel tilt in Docker Desktop; only single-Redis fail-open tested;
- **Long SSE soak**: 3s across churn, not a multi-hour soak;
- **Helm/Envoy binary validation**: tools absent; T8 Docker is the runtime check.

## 6. CI and what green means

Current CI (`.github/workflows/test.yml`):

```text
gofmt
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

Green means Go-level checks pass. It does **not** cover T8, Helm, load, chaos, or
Kubernetes. Latest green: #18 `49e4c54`.

Historical red runs were mostly a flaky `go test -race` test in
`internal/load`; fixed by `6de166f` (mutex around captured headers). Old commit
statuses do not need to be rewritten.

## 7. Distance to production-grade MCP gateway

| Dimension | Gap |
|---|---|
| HA | single control plane/data plane; 3×503 on replica stop; no EDS/draining |
| K8s | CRD controller is fake-tested; no real cluster/admission/status/finalizers |
| Control-plane security | no API auth/RBAC/mTLS; secrets/config files only |
| State | in-memory snapshots; no durable store/backup; no transactional apply |
| Redis | single instance; Sentinel failover blocked |
| Protocol | core 2026 path, not full conformance; SSE is a shim |
| REST | no multipart/async/idempotency; bridge is WIP |
| Observability | basic metrics/trace; no OTel/alerting/SLO dashboards |
| Performance | single-host 835 RPS/10min; no multi-replica capacity plan |
| Ops | Helm skeleton; no upgrade/rollback/DR/canary validation |
| Testing | no MCP conformance suite, no long soak, T8 not in CI |
| Security | no threat model/secret rotation/egress policy/audit retention |

This is not “one feature short”; it is missing a production operations loop.

## 8. Cautions for resume/demo

Do **not** write:

- “production-grade MCP gateway”;
- “supports 500 RPS / p99 < 10ms” without the single-host qualifier;
- “HA / zero downtime” (current 3×503);
- “real Kubernetes CRD controller validated” (fake API only);
- “Redis HA / Sentinel failover” (blocked);
- “full MCP 2026-07-28 conformance” (core path only);
- “T8 is in CI” (it is not).

Recommended wording:

> “Built a B1 dual-era MCP gateway vertical slice: Envoy data plane + Go control
> plane with xDS LDS/CDS/RDS and ACK/NACK, legacy/modern endpoints, and
> native MCP/REST routing. Verified on a real Docker/Envoy T8 harness, including
> Redis ratelimit, mTLS, SSE, xDS churn, chaos and a 10-minute load run.”

## 9. Recommended next steps

Depth-first on the three differentiators, not breadth:

1. **xDS depth**: durable snapshots/audit, control-plane auth/leader election,
   internal listener xDS, EDS, NACK-triggered rollback.
2. **Dual-endpoint depth**: protocol matrix, SSE shim limits/session semantics,
   error compatibility, graceful drain.
3. **REST/native depth**: per-source retry/timeout/circuit, idempotency,
   multipart/async, source health/metrics.
4. **Fallback/persistence**: config store reload, Redis HA or managed Redis,
   startup/readiness/degraded modes.
5. **Production-only validation** (do not put on resume until verified): WSL +
   kind + Helm, real K8s CRD controller, HA zero-loss, conformance/soak.

WSL2 is the recommended next environment: it enables `go test -race`, Helm,
kind, Envoy binary, security scanners, and Linux T8 scripts. Keep the repo on
the WSL ext4 filesystem, not `/mnt/c`.