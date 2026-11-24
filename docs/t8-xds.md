# T8 xDS churn run — 2026-11-25

**Result: PASS** — 100 LDS/CDS/RDS applies while live traffic continued with 0
transport errors, a long-lived SSE connection survived, 175 ACKs, followed by a
NACK injection and recovery.

## Topology

```
loadgen (10 workers, 3s)
  -> envoy-xds :8443 (LDS/CDS/RDS from ADS)
       -> legacy_internal (envoy-legacy:10001/mcp)
       -> modern_facade   (gateway:8080)
cmd/controlplane :18000 (ADS) / :18080 (config API)
```

The public xDS listener is intentionally separate from the static
ext_authz/ratelimit T8 listener; this run exercises **control-plane churn**, not
the policy chain.

## Command

```powershell
powershell -File scripts/t8-xds.ps1 -Changes 100
```

## Result

```text
xDS initialize: 200 session=84 chars
xDS load: requests=11711 errors=0 rps=3903.7 p50=2.25ms p95=4.18ms p99=5.30ms
xDS ACKs before=3 after=175 changes=100
xDS SSE: sse_ok=true tools=5 (checked at every 10th apply)
xDS long-lived SSE: sse_ok=true tools=5 (held 3s across the churn)
T8 XDS PASS: CDS/RDS updates applied with live load
```

- 100 alternating configs change `listenerHeader`, which changes the HCM
  `stat_prefix` and therefore the **LDS listener resource** (CDS/RDS are also
  re-snapshotted);
- Envoy ACKed the updates (ACK 3 → 175);
- 11,711 live requests, 0 errors, ~3.9k RPS on this host;
- SSE handshake + tools/list still works at every 10th apply;
- one SSE connection held for 3s across the churn and still completed tools/list.

## Bug found

The generated HCM had `Rds` without `RouteConfigName`, so the listener stayed
in `warming_state` forever and `/mcp` returned empty replies. Fixed in
`internal/xds/resources.go` by setting `RouteConfigName: "public"`.

## NACK injection

The T8 control plane enables `-test-invalid`, which exposes
`POST /api/v1/inject-invalid`. It publishes an LDS listener whose network filter
is `envoy.filters.network.definitely_invalid`.

```text
xDS NACKs before=0 after=3143
xDS recovery initialize: status=200
```

Envoy rejected the invalid listener, kept the previous good listener, and a
fresh initialize still returned 200. The high NACK count is Envoy repeatedly
rejecting the bad snapshot in this short window, not request failures.

## What this proves

- real ADS with LDS/CDS/RDS;
- Envoy ACK flow during rapid **LDS listener** changes;
- no transport errors in the client during 100 updates;
- Envoy NACKs an invalid listener and keeps serving the last good one;
- recovery after re-applying a valid config.

## Not covered

- multi-node xDS / leader election (this is one control plane and one Envoy);
- multi-node xDS / leader election;
- the policy chain on the xDS listener.

Do not claim full C-11 zero-downtime from this run alone.