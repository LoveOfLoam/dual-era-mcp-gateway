# T8 xDS churn run — 2026-11-25

**Result: PASS** — 100 LDS/CDS/RDS applies while live traffic continued with 0
transport errors and 111 ACKs, followed by a NACK injection and recovery.

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
xDS load: requests=12740 errors=0 rps=4246.6 p50=2.15ms p95=3.69ms p99=4.34ms
xDS ACKs before=3 after=111 changes=100
T8 XDS PASS: CDS/RDS updates applied with live load
```

- 100 alternating configs change `listenerHeader`, which changes the HCM
  `stat_prefix` and therefore the **LDS listener resource** (CDS/RDS are also
  re-snapshotted);
- Envoy ACKed the updates (ACK 3 → 111);
- 12,740 live requests, 0 errors, ~4.2k RPS on this host.

## Bug found

The generated HCM had `Rds` without `RouteConfigName`, so the listener stayed
in `warming_state` forever and `/mcp` returned empty replies. Fixed in
`internal/xds/resources.go` by setting `RouteConfigName: "public"`.

## NACK injection

The T8 control plane enables `-test-invalid`, which exposes
`POST /api/v1/inject-invalid`. It publishes an LDS listener whose network filter
is `envoy.filters.network.definitely_invalid`.

```text
xDS NACKs before=0 after=3054
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

- SSE during churn;
- multi-node xDS / leader election;
- the policy chain on the xDS listener.

Do not claim full C-11 zero-downtime from this run alone.