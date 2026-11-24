# ConfigMap directory controller

`cmd/controller` watches a directory of `McpSource`/`McpConsumer` YAML and
POSTs the compiled `controlplane.Config` to `/api/v1/apply` only when the
content hash changes.

This ConfigMap controller is **not** the CRD controller. A separate
`cmd/crd-controller` + `internal/kube` list/watch implementation exists for
`McpSource`/`McpConsumer`; see `docs/k8s-controller.md` and
`deploy/controller/crd-deployment.yaml`.

Both paths lack leader election, finalizers, and a real-cluster validation in
this environment.

Local run:

```powershell
go run ./cmd/controlplane -addr :18080 -xds-addr :18000
go run ./cmd/controller -dir ./deploy/controller/example -controlplane http://127.0.0.1:18080 -once
```