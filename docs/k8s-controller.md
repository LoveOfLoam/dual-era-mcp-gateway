# CRD controller (list/watch, no client-go)

`cmd/crd-controller` watches the `gateway.dual-era.io/v1alpha1` CRDs
`McpSource` and `McpConsumer`, compiles them into `controlplane.Config`, and
POSTs changes to the control-plane `/api/v1/apply`.

It intentionally uses a small REST client (`internal/kube`) instead of
client-go, keeping the controller image dependency-free.

## Flow

```
McpSource/McpConsumer CRDs
  -> list + watch (Kubernetes API)
  -> ConfigFromObjects
  -> controlplane.Validate
  -> HTTPApplier (/api/v1/apply)
  -> xDS snapshot
```

## Run locally

```powershell
go run ./cmd/crd-controller -api http://127.0.0.1:8001 -namespace default `
  -token-file "" -controlplane http://127.0.0.1:18080 -once
```

Use `kubectl proxy` for the local API endpoint; omit `-token-file` when the
proxy is unauthenticated.

## Tests

`internal/kube/client_test.go` uses a fake Kubernetes API:

- list sources/consumers;
- resourceVersion propagation;
- compile + validate;
- content-hash dedupe (second reconcile does not re-apply).

```powershell
go test ./internal/kube
```

## Deploy

`deploy/controller/crd-deployment.yaml` contains:

- ServiceAccount + ClusterRole/Binding (`get/list/watch` on both CRDs);
- Deployment running `/crd-controller`.

Not validated against a real cluster in this environment (no kind/kubeconfig).
The ConfigMap directory controller remains available as a non-CRD mode.