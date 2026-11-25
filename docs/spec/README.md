# Spec index

Authoritative current status: [`../project-status.md`](../project-status.md).

| Document | Purpose | Freshness |
|---|---|---|
| [`requirements.md`](requirements.md) | functional/non-functional requirements | historical PRD mapping |
| [`protocol-matrix.md`](protocol-matrix.md) | legacy vs modern protocol matrix | mostly current; verify T8 rows |
| [`providers.md`](providers.md) | Provider sources | current |
| [`tool-sources.md`](tool-sources.md) | native MCP / REST tool sources | current |
| [`acceptance-tests.md`](acceptance-tests.md) | T0–T8 acceptance list | **stale T8 rows**; see project-status |
| [`traceability.md`](traceability.md) | FR/AC matrix | **stale**; do not cite as current |
| [`../adr/`](../adr/) | ADR-001..008 | current decisions |
| [`../test-report.md`](../test-report.md) | Go/T8 evidence summary | current with caveats |
| [`../t8-report.md`](../t8-report.md) | T8 runtime evidence | current |
| [`../t8-xds.md`](../t8-xds.md) | xDS churn/NACK | current |
| [`../t8-mtls.md`](../t8-mtls.md) | mTLS | current |
| [`../chaos-report.md`](../chaos-report.md) | failure injection | current |
| [`../blockers.md`](../blockers.md) | environment blockers | current |

## Missing production specs

- SLO/error budget and capacity plan;
- threat model, secret rotation, control-plane RBAC;
- HA/multi-replica and failover design;
- full MCP 2026 conformance matrix;
- control-plane OpenAPI/JSON Schema;
- evidence index per claim (script + result + commit).