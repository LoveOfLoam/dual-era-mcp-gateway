# Observability assets

- `grafana-dashboard.json` — starter dashboard for the counters exposed by
  `cmd/gateway` `/metrics`.
- `prometheus-rules.yaml` — starter alert rules.

These are **assets, not validated dashboards**. They are not wired into a live
Prometheus/Grafana in this environment. Envoy metrics, policy gRPC metrics and
xDS NACK metrics are not yet exposed; the dashboard only covers the Go gateway
counters.

When adding real metrics:

- keep tenant/tool labels bounded or hashed;
- add NACK/control-plane/Redis/cert alerts with runbook links;
- record a screenshot or JSON export in the final report.