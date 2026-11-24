# ConfigMap directory controller

`cmd/controller` watches a directory of `McpSource`/`McpConsumer` YAML and
POSTs the compiled `controlplane.Config` to `/api/v1/apply` only when the
content hash changes.

This is **not** a CRD controller:

- it polls a mounted ConfigMap volume, not the Kubernetes API;
- it has no informer, work queue, finalizer, status/conditions, or leader election;
- it does not read `deploy/crds` objects.

It is a small, testable reconciliation path for the ConfigMap deployment mode.
The CRD controller remains future work (B-10 boundary in `task_plan.md`).

Local run:

```powershell
go run ./cmd/controlplane -addr :18080 -xds-addr :18000
go run ./cmd/controller -dir ./deploy/controller/example -controlplane http://127.0.0.1:18080 -once
```