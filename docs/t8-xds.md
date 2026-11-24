# T8 xDS churn run — 2026-11-25

**Result: PASS** — 100 CDS/RDS applies while live traffic continued with 0
transport errors and 250 ACKs.

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
xDS load: requests=12580 errors=0 rps=4192.6 p50=2.14ms p95=3.71ms p99=4.46ms
xDS ACKs before=3 after=250 changes=100
T8 XDS PASS: CDS/RDS updates applied with live load
```

- 100 alternating valid configs → CDS/RDS snapshots;
- Envoy ACKed the updates (ACK 3 → 250);
- 12,580 live requests, 0 errors, ~4.2k RPS on this host.

## Bug found

The generated HCM had `Rds` without `RouteConfigName`, so the listener stayed
in `warming_state` forever and `/mcp` returned empty replies. Fixed in
`internal/xds/resources.go` by setting `RouteConfigName: "public"`.

## What this proves

- real ADS with CDS/RDS/LDS;
- Envoy ACK flow during rapid config changes;
- no transport errors in the client during 100 CDS/RDS updates;
- xDS listener serves an initialize and tools/list.

## Not covered

- 100 **LDS** listener replacements (only the initial LDS and CDS/RDS churn);
- SSE during churn;
- NACK injection (invalid resource deliberately rejected);
- multi-node xDS / leader election;
- the policy chain on the xDS listener.

Do not claim full C-11 zero-downtime from this run alone.