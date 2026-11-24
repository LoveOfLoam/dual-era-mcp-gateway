# Helm chart — dual-era (skeleton)

This chart is a **deployment skeleton**, not a production-grade chart.

It renders:

- controlplane Deployment + Service (HTTP 18080, ADS 18000);
- gateway Deployment (2 replicas) + Service (8080);
- policy Deployment + Service (ext_authz 19001, ratelimit 19002);
- public Envoy Deployment + LoadBalancer Service (8080);
- internal Envoy Deployment + Service (10001);
- one ConfigMap with `controlplane.json`, `policy.json`, and both Envoy configs.

## Status / limitations

- Not tested against a live cluster in this environment.
- No CRDs/controller/admission webhook yet; `controlplane.json` is file-based.
- No TLS/mTLS, Redis, HPA/PDB, NetworkPolicy, ServiceMonitor, or Ingress.
- The public Envoy uses `ext_authz`; the gateway is deliberately **not** given
  `-policy` in this chart to avoid double authentication after ext_authz removes
  the credential headers. Local `cmd/gateway -policy` is a separate mode.
- Envoy image defaults to `envoyproxy/envoy:dev`; pin a digest for any real run.

## Commands

```powershell
powershell -File scripts/helm-lint.ps1
# or on a cluster:
helm upgrade --install dual-era deploy/helm/dual-era
```